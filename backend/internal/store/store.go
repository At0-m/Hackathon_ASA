package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aca/backend/internal/models"
	"aca/backend/internal/workspace"
)

type Store struct {
	mu       sync.RWMutex
	dataDir  string
	sessions map[string]*models.Session
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o755); err != nil {
		return nil, err
	}
	return &Store{dataDir: dataDir, sessions: map[string]*models.Session{}}, nil
}

func (s *Store) Create(task models.Task) (*models.Session, error) {
	now := time.Now()
	if task.Title == "" {
		task = DefaultTask()
	}
	session := &models.Session{
		ID:             token(8),
		CandidateToken: token(16),
		ReviewerToken:  token(16),
		Task:           task,
		Files:          workspace.DefaultFiles(),
		Steps:          []models.PromptStep{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return session, s.persist(session)
}

func (s *Store) Get(id string) (*models.Session, bool) {
	s.mu.RLock()
	if session, ok := s.sessions[id]; ok {
		copy := cloneSession(session)
		s.mu.RUnlock()
		return &copy, true
	}
	s.mu.RUnlock()

	loaded, err := s.load(id)
	if err != nil {
		return nil, false
	}
	s.mu.Lock()
	s.sessions[id] = loaded
	copy := cloneSession(loaded)
	s.mu.Unlock()
	return &copy, true
}

func (s *Store) Update(id string, fn func(*models.Session) error) (*models.Session, error) {
	s.mu.Lock()
	session, ok := s.sessions[id]
	if !ok {
		var err error
		session, err = s.load(id)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		s.sessions[id] = session
	}
	if err := fn(session); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	session.UpdatedAt = time.Now()
	copy := cloneSession(session)
	s.mu.Unlock()
	return &copy, s.persist(&copy)
}

func (s *Store) Authorize(session *models.Session, tokenValue string, write bool) error {
	if tokenValue == "" {
		return errors.New("missing token")
	}
	if tokenValue == session.CandidateToken {
		return nil
	}
	if !write && tokenValue == session.ReviewerToken {
		return nil
	}
	return errors.New("bad token")
}

func (s *Store) PersistFiles(session *models.Session) error {
	base := filepath.Join(s.dataDir, "sessions", session.ID, "files")
	if err := os.RemoveAll(base); err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	flat := workspace.Flatten(session.Files)
	for p, file := range flat {
		clean := workspace.CleanPath(p)
		if clean == "" {
			continue
		}
		full := filepath.Join(base, filepath.FromSlash(clean))
		if !strings.HasPrefix(full, base) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func DefaultTask() models.Task {
	return models.Task{
		Title:          "AI Interview: проверка AI-operating skill",
		Role:           "Кандидат",
		TimeboxMinutes: 25,
		AllowCode:      true,
		Language:       "ru",
		Instructions: strings.TrimSpace(`За 25 минут нужно получить полезный рабочий артефакт с помощью Алисы. Оценивается не только результат, но и процесс: постановка задачи, декомпозиция, контекст, итерации, проверка ошибок, работа с ограничениями и качество финального артефакта.

Рекомендуемый путь: сначала уточни цель и ограничения, затем попроси Алису собрать первую версию, потом проверь ошибки/edge cases, улучши файлы и сдай работу с коротким self-review.`),
		ExpectedDeliverables: []string{
			"рабочие файлы в workspace",
			"понятный README или описание результата",
			"цепочка промптов",
			"финальное self-review",
			"проверка ограничений и ошибок",
		},
	}
}

func (s *Store) Public(session models.Session, baseURL string, includeTokens bool) models.PublicSession {
	p := models.PublicSession{
		ID:              session.ID,
		Task:            session.Task,
		Files:           session.Files,
		Steps:           session.Steps,
		FinalSubmission: session.FinalSubmission,
		CodeEvaluation:  session.CodeEvaluation,
		Review:          session.Review,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}
	frontendBase := os.Getenv("FRONTEND_BASE_URL")
	if frontendBase == "" {
		frontendBase = "http://localhost:5173"
	}
	p.CandidateURL = fmt.Sprintf("%s/?session=%s&token=%s&role=candidate", strings.TrimRight(frontendBase, "/"), session.ID, session.CandidateToken)
	p.ReviewerURL = fmt.Sprintf("%s/?session=%s&token=%s&role=reviewer", strings.TrimRight(frontendBase, "/"), session.ID, session.ReviewerToken)
	if includeTokens {
		p.CandidateToken = session.CandidateToken
		p.ReviewerToken = session.ReviewerToken
	}
	_ = baseURL
	return p
}

func (s *Store) persist(session *models.Session) error {
	if err := s.PersistFiles(session); err != nil {
		return err
	}
	base := filepath.Join(s.dataDir, "sessions", session.ID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, "session.json"), body, 0o644)
}

func (s *Store) load(id string) (*models.Session, error) {
	body, err := os.ReadFile(filepath.Join(s.dataDir, "sessions", id, "session.json"))
	if err != nil {
		return nil, err
	}
	var session models.Session
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func cloneSession(session *models.Session) models.Session {
	body, _ := json.Marshal(session)
	var out models.Session
	_ = json.Unmarshal(body, &out)
	return out
}

func token(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
