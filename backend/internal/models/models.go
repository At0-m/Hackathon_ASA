package models

import "time"

type Task struct {
	Title                string   `json:"title"`
	Instructions         string   `json:"instructions"`
	Role                 string   `json:"role"`
	TimeboxMinutes       int      `json:"timebox_minutes"`
	ExpectedDeliverables []string `json:"expected_deliverables"`
	AllowCode            bool     `json:"allow_code"`
	Language             string   `json:"language"`
}

type FileNode struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Language string     `json:"language,omitempty"`
	Content  string     `json:"content,omitempty"`
	Children []FileNode `json:"children,omitempty"`
	IsOpen   bool       `json:"isOpen,omitempty"`
}

type FileChange struct {
	Action   string `json:"action"`
	Path     string `json:"path"`
	Language string `json:"language,omitempty"`
	Content  string `json:"content,omitempty"`
}

type PromptStep struct {
	ID            string       `json:"id"`
	Number        int          `json:"number"`
	Prompt        string       `json:"prompt"`
	CandidateNote string       `json:"candidate_note,omitempty"`
	ModelOutput   string       `json:"model_output"`
	ModelProvider string       `json:"model_provider"`
	ModelName     string       `json:"model_name"`
	DurationMS    int64        `json:"duration_ms"`
	OutputHash    string       `json:"output_hash"`
	FileChanges   []FileChange `json:"file_changes,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

type FinalSubmission struct {
	FinalAnswer string     `json:"final_answer"`
	Code        string     `json:"code"`
	SelfReview  string     `json:"self_review"`
	Files       []FileNode `json:"files"`
	SubmittedAt time.Time  `json:"submitted_at"`
}

type CodeEvaluation struct {
	Enabled       bool     `json:"enabled"`
	MistralScore  int      `json:"mistral_score"`
	AliceScore    int      `json:"alice_score"`
	LocalBonus    int      `json:"local_bonus"`
	WeightedScore int      `json:"weighted_score"`
	Summary       string   `json:"summary"`
	Findings      []string `json:"findings"`
}

type Dimension struct {
	Name     string `json:"name"`
	Score    int    `json:"score"`
	MaxScore int    `json:"max_score"`
	Reason   string `json:"reason"`
}

type PromptLabel struct {
	StepNumber int      `json:"step_number"`
	Labels     []string `json:"labels"`
	Notes      string   `json:"notes"`
}

type ReviewReport struct {
	ReportID               string        `json:"report_id"`
	SessionID              string        `json:"session_id"`
	TotalScore             int           `json:"total_score"`
	Dimensions             []Dimension   `json:"dimensions"`
	PromptLabels           []PromptLabel `json:"prompt_labels"`
	Strengths              []string      `json:"strengths"`
	Weaknesses             []string      `json:"weaknesses"`
	RedFlags               []string      `json:"red_flags"`
	Recommendation         string        `json:"recommendation"`
	Summary                string        `json:"summary"`
	NextInterviewQuestions []string      `json:"next_interview_questions"`
	PracticalTasks         []string      `json:"practical_tasks"`
	EthicsPrivacyNotes     []string      `json:"ethics_privacy_notes"`
	GeneratedBy            string        `json:"generated_by"`
}

type Session struct {
	ID              string           `json:"id"`
	CandidateToken  string           `json:"candidate_token,omitempty"`
	ReviewerToken   string           `json:"reviewer_token,omitempty"`
	Task            Task             `json:"task"`
	Files           []FileNode       `json:"files"`
	Steps           []PromptStep     `json:"steps"`
	FinalSubmission *FinalSubmission `json:"final_submission,omitempty"`
	CodeEvaluation  *CodeEvaluation  `json:"code_evaluation,omitempty"`
	Review          *ReviewReport    `json:"review,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type PublicSession struct {
	ID              string           `json:"id"`
	CandidateToken  string           `json:"candidate_token,omitempty"`
	ReviewerToken   string           `json:"reviewer_token,omitempty"`
	CandidateURL    string           `json:"candidate_url,omitempty"`
	ReviewerURL     string           `json:"reviewer_url,omitempty"`
	Task            Task             `json:"task"`
	Files           []FileNode       `json:"files"`
	Steps           []PromptStep     `json:"steps"`
	FinalSubmission *FinalSubmission `json:"final_submission,omitempty"`
	CodeEvaluation  *CodeEvaluation  `json:"code_evaluation,omitempty"`
	Review          *ReviewReport    `json:"review,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type CreateSessionRequest struct {
	Task *Task `json:"task,omitempty"`
}

type SendPromptRequest struct {
	Prompt        string     `json:"prompt"`
	Files         []FileNode `json:"files"`
	SelectedModel string     `json:"selected_model"`
	ActiveFileID  string     `json:"active_file_id"`
}

type SendPromptResponse struct {
	Message     string        `json:"message"`
	Step        PromptStep    `json:"step"`
	FileChanges []FileChange  `json:"file_changes"`
	Session     PublicSession `json:"session"`
}

type SaveFilesRequest struct {
	Files []FileNode `json:"files"`
}

type SubmitRequest struct {
	FinalAnswer string     `json:"final_answer"`
	Code        string     `json:"code"`
	SelfReview  string     `json:"self_review"`
	Files       []FileNode `json:"files"`
}
