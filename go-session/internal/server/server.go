package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/daniel-talonone/gemini-commands/internal/dashboard"
	"github.com/daniel-talonone/gemini-commands/internal/description"
	"github.com/daniel-talonone/gemini-commands/internal/llm"
	"github.com/daniel-talonone/gemini-commands/internal/log"
	"github.com/daniel-talonone/gemini-commands/internal/plan"
	"github.com/daniel-talonone/gemini-commands/internal/review"
	"github.com/daniel-talonone/gemini-commands/internal/status"
)

//go:embed template.html
var templateHTML string

// PageData is passed to the HTML template on every request.
type PageData struct {
	Features     []dashboard.FeatureState
	AllRepos     []string
	RepoFilter   string
	StatusFilter string
	SortBy       string
	SortOrder    string
}

// FeatureDetailData is passed to the feature_detail template.
type FeatureDetailData struct {
	ID          string
	Description template.HTML
	Log         template.HTML
	Repo        string
	Branch      string
	PRURL       string
	StoryURL    string
	WorkDir     string
	Plan        plan.Plan

	// ReviewFiles holds the discovered review type names (e.g., "", "docs", "devops").
	// Populated by the detail handler via review.DiscoverTypes.
	ReviewFiles []string

	// SelectedReview is the review type name currently selected in the UI.
	// Populated from the "review" query parameter.
	SelectedReview string

	// Reviews holds the findings for the selected review type.
	// Populated by the detail handler by loading the corresponding review file.
	Reviews []review.Finding

	// HasOpenFindings is true if any finding in the selected review has status "open".
	// Populated by the detail handler after loading findings.
	HasOpenFindings bool

	// PipelineStep is the current pipeline_step from status.yaml.
	// Used to render the correct button state on page load.
	PipelineStep string
}

// Server is the dashboard HTTP server.
//go:generate mockgen -source=server.go -destination=mock_server.go -package=server
type Scanner interface {
	ScanAll() ([]dashboard.FeatureState, error)
}

// DashboardScanner implements the server.Scanner interface using dashboard.ScanAll.
type DashboardScanner struct{}

func (ds *DashboardScanner) ScanAll() ([]dashboard.FeatureState, error) {
	return dashboard.ScanAll()
}

type Server struct {
	port        int
	http        *http.Server
	ScanAllFunc func() ([]dashboard.FeatureState, error)
	hubs        map[string]*FeatureHub
	hubsMu      sync.Mutex
	cancels     map[string]context.CancelFunc
	cancelsMu   sync.Mutex
	tmpl        *template.Template
}

// New creates a new Server listening on the given port.
func New(port int, scanner Scanner) *Server {
	return &Server{
		port:        port,
		ScanAllFunc: scanner.ScanAll,
		hubs:        make(map[string]*FeatureHub),
		cancels:     make(map[string]context.CancelFunc),
	}
}

func (s *Server) storePlanCancel(id string, cancel context.CancelFunc) {
	s.cancelsMu.Lock()
	s.cancels[id] = cancel
	s.cancelsMu.Unlock()
}

func (s *Server) removePlanCancel(id string) {
	s.cancelsMu.Lock()
	delete(s.cancels, id)
	s.cancelsMu.Unlock()
}

func (s *Server) cancelPlan(id string) {
	s.cancelsMu.Lock()
	if cancel, ok := s.cancels[id]; ok {
		cancel()
		delete(s.cancels, id)
	}
	s.cancelsMu.Unlock()
}

func (s *Server) getOrCreateHub(id string) *FeatureHub {
	s.hubsMu.Lock()
	defer s.hubsMu.Unlock()
	if h, ok := s.hubs[id]; ok {
		return h
	}
	h := NewFeatureHub()
	s.hubs[id] = h
	return h
}

// Start parses the template, registers routes, and begins listening.
// Blocks until the server stops. Returns nil on clean shutdown (ErrServerClosed).
func (s *Server) Start() error {
	tmpl, err := template.New("dashboard").Funcs(template.FuncMap{
		"safeURL": func(s string) template.URL { return template.URL(s) },
	}).Parse(templateHTML)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	s.tmpl = tmpl

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.MakeListHandler(tmpl))
	mux.HandleFunc("/feature/", func(w http.ResponseWriter, r *http.Request) {
		pathSuffix := strings.TrimPrefix(r.URL.Path, "/feature/")
		if strings.HasSuffix(pathSuffix, "/reset") && r.Method == http.MethodPost {
			s.MakeResetHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/plan") && r.Method == http.MethodPost {
			s.MakePlanHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/events") && r.Method == http.MethodGet {
			s.MakeFeatureEventsHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/plan-section") && r.Method == http.MethodGet {
			s.MakePlanSectionHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/plan-area") && r.Method == http.MethodGet {
			s.MakePlanAreaHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/plan-stop") && r.Method == http.MethodPost {
			s.MakePlanStopHandler()(w, r)
		} else if strings.HasSuffix(pathSuffix, "/clear") && r.Method == http.MethodPost {
			s.MakeClearHandler()(w, r)
		} else {
			s.MakeFeatureDetailHandler(tmpl)(w, r)
		}
	})
	mux.HandleFunc("/action/terminal", TerminalHandler)
	mux.HandleFunc("/action/finder", FinderHandler)

	addr := fmt.Sprintf(":%d", s.port)
	s.http = &http.Server{Addr: addr, Handler: mux}

	fmt.Printf("Dashboard running at http://localhost%s\n", addr)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully drains in-flight requests then stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// TerminalHandler handles GET /action/terminal?path=<dir> by opening a new
// Terminal.app window at the given directory. Returns 400 if path is missing
// or not an existing directory, 500 if the open command fails, 204 on success.
func TerminalHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		http.Error(w, "path is not an existing directory", http.StatusBadRequest)
		return
	}
	if err := exec.Command("open", "-a", "Terminal", path).Run(); err != nil {
		http.Error(w, "failed to open terminal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// FinderHandler handles GET /action/finder?path=<dir> by opening the directory
// in Finder. Returns 400 if path is missing or not an existing directory,
// 500 if the open command fails, 204 on success.
func FinderHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		http.Error(w, "path is not an existing directory", http.StatusBadRequest)
		return
	}
	if err := exec.Command("open", path).Run(); err != nil {
		http.Error(w, "failed to open finder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


func (s *Server) MakeResetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only handle POST requests to paths ending with /reset
		pathSuffix := strings.TrimPrefix(r.URL.Path, "/feature/")
		if r.Method != http.MethodPost || !strings.HasSuffix(pathSuffix, "/reset") {
			http.NotFound(w, r)
			return
		}

		// Extract feature ID by removing /reset suffix
		id := strings.TrimSuffix(pathSuffix, "/reset")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		// Scan all features to find the one with matching ID
		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		dir := found.Dir
		if dir == "" {
			http.Error(w, "feature directory not found", http.StatusNotFound)
			return
		}

		// Load and validate plan is not empty
		pln, err := plan.LoadPlan(dir)
		if err != nil || len(pln) == 0 {
			http.Error(w, "plan is empty or missing", http.StatusInternalServerError)
			return
		}

		// Reset all task and slice statuses to "todo"
		if err := plan.ResetPlan(dir); err != nil {
			http.Error(w, "failed to reset plan: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Clear pipeline_step in status.yaml
		if err := status.Write(dir, "", "", ""); err != nil {
			http.Error(w, "failed to clear pipeline_step: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Return plan_section fragment for HTMX; fall back to redirect for plain requests.
		if r.Header.Get("HX-Request") != "true" {
			http.Redirect(w, r, "/feature/"+id, http.StatusSeeOther)
			return
		}

		data := FeatureDetailData{ID: id}
		if pln, err := plan.LoadPlan(dir); err == nil {
			data.Plan = pln
		}
		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, "plan_section", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

func (s *Server) MakePlanHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract feature ID by removing /plan suffix
		pathSuffix := strings.TrimPrefix(r.URL.Path, "/feature/")
		id := strings.TrimSuffix(pathSuffix, "/plan")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		// Scan all features to find the one with matching ID
		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		// Use the directory recorded by the scanner — no re-resolution needed.
		dir := found.Dir
		if dir == "" {
			http.Error(w, "feature directory not found", http.StatusNotFound)
			return
		}

		// Load plan only if plan.yml exists; a missing file means no plan yet.
		if _, statErr := os.Stat(filepath.Join(dir, "plan.yml")); statErr == nil {
			pln, err := plan.LoadPlan(dir)
			if err != nil {
				http.Error(w, "failed to load plan: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if len(pln) > 0 {
				http.Error(w, "plan already exists", http.StatusConflict)
				return
			}
		}

		// Register a hub for this feature and start the plan goroutine.
		h := s.getOrCreateHub(id)
		model := llm.Model(r.FormValue("model"))
		switch model {
		case llm.ModelGemini, llm.ModelGeminiFlash, llm.ModelClaude:
			// valid
		default:
			model = llm.ModelGemini
		}

		runner, err := llm.NewRunner(model, llm.RunnerOptions{})
		if err != nil {
			http.Error(w, "runner error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ctx, cancelFn := context.WithCancel(context.Background())
		s.storePlanCancel(id, cancelFn)

		go func() {
			defer s.removePlanCancel(id)
			fmt.Printf("[plan] starting: feature=%s model=%s dir=%s\n", id, model, dir)
			runErr := plan.RunSkipEnrich(ctx, id, dir, runner, func(msg string) {
				fmt.Printf("[plan] %s: %s\n", id, msg)
				h.Publish(Event{Type: "progress", Message: msg})
			})
			if runErr != nil {
				fmt.Printf("[plan] failed: feature=%s error=%v\n", id, runErr)
				h.Publish(Event{Type: "failed", Message: runErr.Error()})
			} else {
				fmt.Printf("[plan] done: feature=%s\n", id)
				h.Publish(Event{Type: "done", Message: "Plan ready"})
			}
			h.Close()
			// Evict the hub after a grace period so late-connecting SSE clients fall
			// through to the status.yaml fallback path.
			time.AfterFunc(30*time.Second, func() {
				s.hubsMu.Lock()
				delete(s.hubs, id)
				s.hubsMu.Unlock()
			})
		}()

		// HTMX request: return the plan_area fragment directly so the browser
		// can swap it in without a full page reload.
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			data := FeatureDetailData{ID: id, PipelineStep: "plan"}
			var buf bytes.Buffer
			if err := s.tmpl.ExecuteTemplate(&buf, "plan_area", data); err != nil {
				http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(buf.Bytes())
			return
		}

		// Regular form submit: redirect to feature detail page.
		http.Redirect(w, r, "/feature/"+id, http.StatusSeeOther)
	}
}

// MakeFeatureEventsHandler serves GET /feature/{id}/events as an SSE stream.
// While a plan job is active for the feature, it forwards events from the hub.
// Once the hub is gone (job finished + grace period elapsed), it sends one
// synthetic "status" event derived from status.yaml so reconnecting clients
// can settle into the correct UI state.
func (s *Server) MakeFeatureEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/feature/"), "/events")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		sendJSON := func(e Event) {
			b, _ := json.Marshal(e)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}

		s.hubsMu.Lock()
		h, active := s.hubs[id]
		s.hubsMu.Unlock()

		if !active {
			// Job already finished or never started — synthesise from status.yaml.
			features, err := s.ScanAllFunc()
			if err != nil {
				sendJSON(Event{Type: "failed", Message: "scan error: " + err.Error()})
				return
			}
			var found *dashboard.FeatureState
			for i := range features {
				if features[i].StoryID == id {
					found = &features[i]
					break
				}
			}
			if found == nil {
				sendJSON(Event{Type: "failed", Message: "feature not found"})
				return
			}
			dir := found.Dir
			if dir == "" {
				sendJSON(Event{Type: "failed", Message: "feature directory not found"})
				return
			}
			st, err := status.LoadStatus(dir)
			if err != nil {
				sendJSON(Event{Type: "failed", Message: "status error: " + err.Error()})
				return
			}
			sendJSON(Event{Type: "status", Step: st.PipelineStep})
			return
		}

		ch, unsub := h.Subscribe()
		defer unsub()

		for {
			select {
			case e, ok := <-ch:
				if !ok {
					return
				}
				sendJSON(e)
				if e.Type == "done" || e.Type == "failed" {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

// MakePlanSectionHandler serves GET /feature/{id}/plan-section as an HTML
// fragment containing the plan button area and plan details. Used by the HTMX
// swap after plan generation completes.
func (s *Server) MakePlanSectionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/feature/"), "/plan-section")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		dir := found.Dir
		if dir == "" {
			http.NotFound(w, r)
			return
		}

		data := FeatureDetailData{ID: id}
		if st, err := status.LoadStatus(dir); err == nil {
			data.PipelineStep = st.PipelineStep
		}
		if pln, err := plan.LoadPlan(dir); err == nil {
			data.Plan = pln
		}

		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, "plan_section", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

// MakePlanAreaHandler serves GET /feature/{id}/plan-area as an HTML fragment
// containing only the plan button area. Used by the SSE failure handler to
// re-render the split button without a full page reload.
func (s *Server) MakePlanAreaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/feature/"), "/plan-area")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		dir := found.Dir
		if dir == "" {
			http.NotFound(w, r)
			return
		}

		data := FeatureDetailData{ID: id}
		if st, err := status.LoadStatus(dir); err == nil {
			data.PipelineStep = st.PipelineStep
		}

		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, "plan_area", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

// MakePlanStopHandler handles POST /feature/{id}/plan-stop. It cancels the
// in-flight plan goroutine for the feature and returns the idle plan_area fragment.
func (s *Server) MakePlanStopHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/feature/"), "/plan-stop")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Printf("[plan] stop requested: feature=%s\n", id)
		s.cancelPlan(id)

		data := FeatureDetailData{ID: id}
		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, "plan_area", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

// MakeClearHandler handles POST /feature/{id}/clear. It removes plan.yml,
// architecture.md, and questions.yml from the feature directory, appends a log
// entry, and returns the plan_section fragment so the UI refreshes without reload.
func (s *Server) MakeClearHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathSuffix := strings.TrimPrefix(r.URL.Path, "/feature/")
		id := strings.TrimSuffix(pathSuffix, "/clear")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		dir := found.Dir
		if dir == "" {
			http.NotFound(w, r)
			return
		}

		for _, name := range []string{"plan.yml", "architecture.md", "questions.yml"} {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				http.Error(w, "failed to remove "+name+": "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err := status.Write(dir, "", "", ""); err != nil {
			http.Error(w, "failed to clear pipeline_step: "+err.Error(), http.StatusInternalServerError)
			return
		}

		_ = log.AppendLog(dir, "Plan cleared via dashboard (plan.yml, architecture.md, questions.yml removed).")

		if r.Header.Get("HX-Request") != "true" {
			http.Redirect(w, r, "/feature/"+id, http.StatusSeeOther)
			return
		}

		data := FeatureDetailData{ID: id}
		var buf bytes.Buffer
		if err := s.tmpl.ExecuteTemplate(&buf, "plan_section", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

func (s *Server) MakeListHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		repoFilter := r.URL.Query().Get("repo")
		statusFilter := r.URL.Query().Get("status")
		sortParam := r.URL.Query().Get("sort")
		orderParam := r.URL.Query().Get("order")

		// Default sort by UpdatedAt descending.
		if sortParam == "" {
			sortParam = "updated"
		}
		if orderParam == "" {
			orderParam = "desc"
		}

		// Reject unknown status filter values.
		if statusFilter != "" && statusFilter != "running" && statusFilter != "done" && statusFilter != "idle" {
			http.Error(w, "invalid status filter: use running, idle, or done", http.StatusBadRequest)
			return
		}

		// Collect unique repos for the filter dropdown.
		repoSet := map[string]struct{}{}
		for _, f := range features {
			if f.Repo != "" {
				repoSet[f.Repo] = struct{}{}
			}
		}
		allRepos := make([]string, 0, len(repoSet))
		for repo := range repoSet {
			allRepos = append(allRepos, repo)
		}
		sort.Strings(allRepos)

		// Apply filters.
		filtered := make([]dashboard.FeatureState, 0, len(features))
		for _, f := range features {
			if repoFilter != "" && f.Repo != repoFilter {
				continue
			}
			if statusFilter != "" {
				switch statusFilter {
				case "running":
					if !f.IsRunning {
						continue
					}
				case "done":
					if !f.AllDone {
						continue
					}
				case "idle":
					if f.IsRunning || f.AllDone {
						continue
					}
				}
			}
			filtered = append(filtered, f)
		}

		// Apply sorting
		SortFeatures(filtered, sortParam, orderParam)

		data := PageData{
			Features:     filtered,
			AllRepos:     allRepos,
			RepoFilter:   repoFilter,
			StatusFilter: statusFilter,
			SortBy:       sortParam,
			SortOrder:    orderParam,
		}

		// Buffer template output — prevents partial responses on error.
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

func (s *Server) MakeFeatureDetailHandler(tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/feature/")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		features, err := s.ScanAllFunc()
		if err != nil {
			http.Error(w, "scan error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var found *dashboard.FeatureState
		for i := range features {
			if features[i].StoryID == id {
				found = &features[i]
				break
			}
		}
		if found == nil {
			http.NotFound(w, r)
			return
		}

		dir := found.Dir
		if dir == "" {
			http.NotFound(w, r)
			return
		}

		desc, _ := description.LoadDescription(dir)

		data := FeatureDetailData{ID: id, Description: description.RenderMarkdown(desc), Repo: found.Repo}

		// Load and render log
		logContent, _ := log.LoadLog(dir)
		data.Log = description.RenderMarkdown(logContent)

		if st, err := status.LoadStatus(dir); err == nil {
			data.Branch = st.Branch
			data.PRURL = st.PRURL
			data.StoryURL = st.StoryURL
			data.WorkDir = st.WorkDir
			data.PipelineStep = st.PipelineStep
		}
		if pln, err := plan.LoadPlan(dir); err == nil {
			data.Plan = pln
		}

		// Load review files
		reviewTypes, err := review.DiscoverTypes(dir)
		if err != nil {
			http.Error(w, "error discovering review files: "+err.Error(), http.StatusInternalServerError)
			return
		}

		selectedReviewType := r.URL.Query().Get("review")

		// Add selected type to list if not present, so dropdown shows it
		if selectedReviewType != "" {
			found := false
			for _, rt := range reviewTypes {
				if rt == selectedReviewType {
					found = true
					break
				}
			}
			if !found {
				reviewTypes = append(reviewTypes, selectedReviewType)
				sort.Strings(reviewTypes)
			}
		}
		data.ReviewFiles = reviewTypes
		data.SelectedReview = selectedReviewType

		var reviewFilename string
		if len(data.ReviewFiles) > 0 {
			reviewName := "review"
			if selectedReviewType != "" {
				reviewName = "review-" + selectedReviewType
			}

			// Try .yml first, then .yaml
			ymlPath := filepath.Join(dir, reviewName+".yml")
			if _, err := os.Stat(ymlPath); err == nil {
				reviewFilename = reviewName + ".yml"
			} else {
				yamlPath := filepath.Join(dir, reviewName+".yaml")
				if _, err := os.Stat(yamlPath); err == nil {
					reviewFilename = reviewName + ".yaml"
				}
			}
		}

		if reviewFilename != "" {
			findings, err := review.LoadByFilename(dir, reviewFilename)
			if err != nil {
				http.Error(w, "error loading review findings: "+err.Error(), http.StatusInternalServerError)
				return
			}
			data.Reviews = findings
			for _, f := range findings {
				if f.Status == "open" {
					data.HasOpenFindings = true
					break
				}
			}
		}

		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "feature_detail", data); err != nil {
			http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	}
}

// sortFeatures sorts the provided feature list in place based on the given
// sort key and direction. It handles invalid or missing timestamps by treating
// them as the oldest possible time.
func SortFeatures(features []dashboard.FeatureState, sortBy, sortDir string) {
	sort.Slice(features, func(i, j int) bool {
		t1 := features[i].UpdatedAt
		t2 := features[j].UpdatedAt
		if sortBy == "started" {
			t1 = features[i].StartedAt
			t2 = features[j].StartedAt
		}

		t1IsZero := t1.IsZero()
		t2IsZero := t2.IsZero()

		if t1IsZero && t2IsZero {
			return false // Treat as equal
		}
		if t1IsZero {
			// Zero time is "smallest", so it comes first in "asc"
			return sortDir != "desc"
		}
		if t2IsZero {
			// Zero time is "smallest", so it comes first in "asc"
			return sortDir == "desc"
		}

		if sortDir == "asc" {
			return t1.Before(t2)
		}
		// Default to "desc"
		return t1.After(t2)
	})
}
