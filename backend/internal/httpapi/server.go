package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"aca/backend/internal/ai"
	"aca/backend/internal/analyzer"
	"aca/backend/internal/config"
	"aca/backend/internal/models"
	"aca/backend/internal/scoring"
	"aca/backend/internal/store"
	"aca/backend/internal/workspace"
)

type Server struct {
	cfg      config.Config
	store    *store.Store
	exec     *ai.Executor
	scorer   *scoring.Service
	analyzer *analyzer.Client
}

func New(cfg config.Config, st *store.Store, exec *ai.Executor, scorer *scoring.Service, an *analyzer.Client) *Server {
	return &Server{cfg: cfg, store: st, exec: exec, scorer: scorer, analyzer: an}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSession)
	mux.HandleFunc("/alice", s.handleAlice)
	return withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	var req models.CreateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	task := store.DefaultTask()
	if req.Task != nil {
		task = *req.Task
	}
	session, err := s.store.Create(task)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, s.store.Public(*session, s.cfg.PublicBaseURL, true))
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("missing session id"))
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	session, ok := s.store.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}
	write := r.Method != http.MethodGet
	if err := s.store.Authorize(session, tokenFrom(r), write); err != nil {
		writeJSON(w, http.StatusUnauthorized, errorBody(err.Error()))
		return
	}
	switch action {
	case "":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, s.store.Public(*session, s.cfg.PublicBaseURL, true))
	case "prompt":
		s.handlePrompt(w, r, id)
	case "files":
		s.handleFiles(w, r, id)
	case "submit":
		s.handleSubmit(w, r, id)
	case "analyze":
		s.handleAnalyze(w, r, id)
	case "review":
		s.handleReview(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, errorBody("unknown route"))
	}
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	var req models.SendPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad json"))
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("empty prompt"))
		return
	}
	current, _ := s.store.Get(id)
	files := current.Files
	if len(req.Files) > 0 {
		files = req.Files
	}
	stepNumber := len(current.Steps) + 1
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	start := time.Now()
	result, err := s.exec.Execute(ctx, prompt, files, stepNumber, req.SelectedModel)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	duration := time.Since(start).Milliseconds()
	updated, err := s.store.Update(id, func(session *models.Session) error {
		if len(req.Files) > 0 {
			session.Files = req.Files
		}
		session.Files = workspace.ApplyChanges(session.Files, result.FileChanges)
		step := models.PromptStep{
			ID:            ai.Hash(id + prompt + time.Now().String()),
			Number:        stepNumber,
			Prompt:        prompt,
			ModelOutput:   result.Raw,
			ModelProvider: result.Provider,
			ModelName:     result.Model,
			DurationMS:    duration,
			OutputHash:    ai.Hash(result.Raw),
			FileChanges:   result.FileChanges,
			CreatedAt:     time.Now(),
		}
		if result.Raw == "" {
			step.ModelOutput = result.Message
		}
		session.Steps = append(session.Steps, step)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	step := updated.Steps[len(updated.Steps)-1]
	writeJSON(w, http.StatusOK, models.SendPromptResponse{Message: result.Message, Step: step, FileChanges: result.FileChanges, Session: s.store.Public(*updated, s.cfg.PublicBaseURL, true)})
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	var req models.SaveFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad json"))
		return
	}
	updated, err := s.store.Update(id, func(session *models.Session) error {
		session.Files = req.Files
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, s.store.Public(*updated, s.cfg.PublicBaseURL, true))
}

func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	var req models.SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("bad json"))
		return
	}
	updated, err := s.store.Update(id, func(session *models.Session) error {
		if len(req.Files) > 0 {
			session.Files = req.Files
		}
		if strings.TrimSpace(req.Code) == "" {
			req.Code = workspace.CollectCode(session.Files)
		}
		if strings.TrimSpace(req.FinalAnswer) == "" {
			req.FinalAnswer = buildFinalAnswer(session.Files)
		}
		session.FinalSubmission = &models.FinalSubmission{FinalAnswer: req.FinalAnswer, Code: req.Code, SelfReview: req.SelfReview, Files: session.Files, SubmittedAt: time.Now()}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	codeEval := s.scorer.EvaluateCode(ctx, *updated)
	updated, err = s.store.Update(id, func(session *models.Session) error {
		session.CodeEvaluation = codeEval
		review := s.analyzer.Analyze(ctx, *session)
		session.Review = &review
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, s.store.Public(*updated, s.cfg.PublicBaseURL, true))
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	current, _ := s.store.Get(id)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	updated, err := s.store.Update(id, func(session *models.Session) error {
		if session.CodeEvaluation == nil {
			session.CodeEvaluation = s.scorer.EvaluateCode(ctx, *session)
		}
		report := s.analyzer.Analyze(ctx, *session)
		if report.TotalScore == 0 {
			return errors.New("empty report")
		}
		session.Review = &report
		return nil
	})
	_ = current
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, s.store.Public(*updated, s.cfg.PublicBaseURL, true))
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	session, ok := s.store.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody("session not found"))
		return
	}
	if session.Review == nil {
		review := scoring.FallbackReview(*session)
		session.Review = &review
	}
	writeJSON(w, http.StatusOK, session.Review)
}

func (s *Server) handleAlice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"response": map[string]any{"text": "ACA работает через web-интерфейс. Откройте ссылку кандидата или ревьюера.", "end_session": false},
		"version":  "1.0",
	})
}

func tokenFrom(r *http.Request) string {
	if value := r.URL.Query().Get("token"); value != "" {
		return value
	}
	if value := r.Header.Get("X-ACA-Token"); value != "" {
		return value
	}
	bearer := r.Header.Get("Authorization")
	return strings.TrimPrefix(bearer, "Bearer ")
}

func buildFinalAnswer(files []models.FileNode) string {
	flat := workspace.Flatten(files)
	if f, ok := flat["README.md"]; ok && strings.TrimSpace(f.Content) != "" {
		return f.Content
	}
	return "Финальный артефакт собран в workspace."
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-ACA-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func errorBody(message string) map[string]string { return map[string]string{"error": message} }
