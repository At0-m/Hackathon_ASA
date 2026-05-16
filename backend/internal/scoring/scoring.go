package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aca/backend/internal/ai"
	"aca/backend/internal/models"
)

type Service struct {
	executor *ai.Executor
}

func New(executor *ai.Executor) *Service {
	return &Service{executor: executor}
}

func (s *Service) EvaluateCode(ctx context.Context, session models.Session) *models.CodeEvaluation {
	code := collectSubmittedCode(session)
	if strings.TrimSpace(code) == "" {
		return nil
	}
	mScore, mFindings := s.judgeCode(ctx, "mistral", code)
	aScore, aFindings := s.judgeCode(ctx, "alice", code)
	if mScore == 0 {
		mScore = localCodeScore(code)
		mFindings = []string{"Локальная проверка: оценка по структуре, наличию обработчиков, обработке ошибок и читаемости."}
	}
	if aScore == 0 {
		aScore = localCodeScore(code) - 4
		if aScore < 0 {
			aScore = 0
		}
		aFindings = []string{"Локальная вторая оценка: проверены базовые признаки полезного кода."}
	}
	bonus := localBonus(session)
	weighted := int(float64(mScore)*0.7 + float64(aScore)*0.3 + float64(bonus))
	if weighted > 100 {
		weighted = 100
	}
	findings := append(mFindings, aFindings...)
	if bonus > 0 {
		findings = append(findings, fmt.Sprintf("Локальный бонус %d за полезные дополнительные файлы или тесты.", bonus))
	}
	return &models.CodeEvaluation{
		Enabled:       true,
		MistralScore:  mScore,
		AliceScore:    aScore,
		LocalBonus:    bonus,
		WeightedScore: weighted,
		Summary:       fmt.Sprintf("Кодовая часть оценена на %d/100: 70%% веса Mistral, 30%% веса Алиса, плюс локальный бонус за полезные артефакты.", weighted),
		Findings:      findings,
	}
}

func FallbackReview(session models.Session) models.ReviewReport {
	labels := classifySteps(session.Steps)
	dimensions := []models.Dimension{
		dimTaskUnderstanding(session, labels),
		dimDecomposition(session, labels),
		dimContext(session, labels),
		dimIteration(session, labels),
		dimVerification(session, labels),
		dimArtifact(session),
		dimEthics(session, labels),
		dimUtility(session),
	}
	total := 0
	for _, d := range dimensions {
		total += d.Score
	}
	total = applyCaps(total, session, labels)
	strengths, weaknesses, redFlags := buildFindings(session, labels, dimensions)
	return models.ReviewReport{
		ReportID:       "report-" + session.ID,
		SessionID:      session.ID,
		TotalScore:     total,
		Dimensions:     dimensions,
		PromptLabels:   promptLabels(session.Steps, labels),
		Strengths:      strengths,
		Weaknesses:     weaknesses,
		RedFlags:       redFlags,
		Recommendation: recommendation(total, redFlags),
		Summary:        summary(total, session, labels),
		NextInterviewQuestions: []string{
			"Как вы поняли, что результат модели действительно решает исходную задачу?",
			"Какие ограничения вы явно дали модели, а какие забыли указать?",
			"Какие части результата вы проверяли вручную и почему?",
			"Где в решении могли появиться галлюцинации или ложная уверенность?",
			"Что бы вы улучшили в prompt-chain, если бы было ещё 10 минут?",
		},
		PracticalTasks: []string{
			"За 7 минут добавьте один тест или smoke-check к полученному артефакту.",
			"Попросите модель найти 3 риска текущего решения и вручную подтвердите или отклоните каждый риск.",
			"Сделайте второй промпт так, чтобы он явно ограничивал формат, критерии успеха и недопустимые предположения.",
		},
		EthicsPrivacyNotes: []string{
			"Оценка построена по действиям кандидата в этой сессии, а не по личным характеристикам.",
			"Сильная работа с ИИ должна отделять проверенные факты от предположений модели.",
			"Финальная рекомендация не заменяет человеческую ответственность за найм, но может использоваться как автоматизированный screening-сигнал.",
		},
		GeneratedBy: "aca-analyzer",
	}
}

func (s *Service) ImproveReviewWithModels(ctx context.Context, session models.Session, base models.ReviewReport) models.ReviewReport {
	payload, _ := json.MarshalIndent(map[string]any{"session": session, "base_review": base}, "", "  ")
	system := `Ты независимый оценщик AI-operating skill. Пиши на русском. Верни строго JSON ReviewReport с теми же полями. Будь строгим: 1-2 общих промпта не могут получать высокий балл. Не оценивай личность кандидата, оценивай только процесс и артефакт.`
	user := "Перепроверь автоматическую оценку и сделай формулировки понятнее. Сохрани строгие cap-правила. Данные:\n" + string(payload)
	text, _, err := s.executor.JudgeText(ctx, "mistral", system, user, "")
	if err != nil || strings.TrimSpace(text) == "" {
		return base
	}
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return base
	}
	var out models.ReviewReport
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		return base
	}
	if out.TotalScore <= 0 || len(out.Dimensions) == 0 {
		return base
	}
	out.TotalScore = applyCaps(out.TotalScore, session, classifySteps(session.Steps))
	out.GeneratedBy = "aca-model-review"
	if out.ReportID == "" {
		out.ReportID = base.ReportID
	}
	if out.SessionID == "" {
		out.SessionID = session.ID
	}
	return out
}

func collectSubmittedCode(session models.Session) string {
	if session.FinalSubmission != nil && strings.TrimSpace(session.FinalSubmission.Code) != "" {
		return session.FinalSubmission.Code
	}
	var b strings.Builder
	for _, file := range flattenFiles(session.Files) {
		if isCodeFile(file.Name) {
			b.WriteString("\n\n// FILE: ")
			b.WriteString(file.Name)
			b.WriteString("\n")
			b.WriteString(file.Content)
		}
	}
	return b.String()
}

func (s *Service) judgeCode(ctx context.Context, provider string, code string) (int, []string) {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	system := `Ты оцениваешь кодовый артефакт кандидата. Пиши на русском. Верни JSON: {"score": 0-100, "findings": ["..."]}. Оцени рабочесть, простоту, обработку ошибок, структуру, соответствие задаче. Будь строгим.`
	user := "Код:\n" + code
	text, _, err := s.executor.JudgeText(ctx, provider, system, user, "")
	if err != nil {
		return 0, nil
	}
	jsonText := extractJSONObject(text)
	if jsonText == "" {
		return 0, nil
	}
	var out struct {
		Score    int      `json:"score"`
		Findings []string `json:"findings"`
	}
	if err := json.Unmarshal([]byte(jsonText), &out); err != nil {
		return 0, nil
	}
	if out.Score < 0 {
		out.Score = 0
	}
	if out.Score > 100 {
		out.Score = 100
	}
	return out.Score, out.Findings
}

func localCodeScore(code string) int {
	low := strings.ToLower(code)
	score := 20
	if strings.Contains(low, "http") || strings.Contains(low, "server") || strings.Contains(low, "handle") || strings.Contains(low, "fastapi") {
		score += 20
	}
	if strings.Contains(low, "error") || strings.Contains(low, "try") || strings.Contains(low, "status") || strings.Contains(low, "method") {
		score += 15
	}
	if strings.Contains(low, "test") || strings.Contains(low, "assert") {
		score += 10
	}
	if len(code) > 400 {
		score += 10
	}
	if strings.Contains(low, "readme") {
		score += 5
	}
	if score > 82 {
		score = 82
	}
	return score
}

func localBonus(session models.Session) int {
	flat := flattenFiles(session.Files)
	hasReadme := false
	hasTest := false
	hasDocker := false
	for _, f := range flat {
		name := strings.ToLower(f.Name)
		if name == "readme.md" {
			hasReadme = true
		}
		if strings.Contains(name, "test") {
			hasTest = true
		}
		if name == "dockerfile" {
			hasDocker = true
		}
	}
	bonus := 0
	if hasReadme {
		bonus += 3
	}
	if hasTest {
		bonus += 5
	}
	if hasDocker {
		bonus += 2
	}
	return bonus
}

type labelSet map[string]bool

func classifySteps(steps []models.PromptStep) []labelSet {
	out := make([]labelSet, len(steps))
	for i, step := range steps {
		low := normalize(step.Prompt + " " + step.ModelOutput)
		labels := labelSet{}
		addIf := func(label string, words ...string) {
			for _, w := range words {
				if strings.Contains(low, w) {
					labels[label] = true
					return
				}
			}
		}
		addIf("clarification", "огранич", "критер", "цель", "формат", "услов", "требован")
		addIf("planning", "разбей", "план", "этап", "архитект", "структур", "шаг")
		addIf("context_setting", "контекст", "роль", "ты", "представь", "вход", "выход", "формат")
		addIf("implementation", "код", "реализ", "сервис", "функц", "endpoint", "api", "файл")
		addIf("iteration", "улучш", "исправ", "перепиши", "доработ", "рефактор")
		addIf("verification", "проверь", "ошиб", "edge", "test", "тест", "валид", "галлюцин", "риск")
		addIf("ethics", "bias", "privacy", "этик", "приват", "личн", "предвз")
		addIf("finalization", "финал", "итог", "собери", "readme", "сдай")
		if len(labels) == 0 {
			labels["unclassified"] = true
		}
		out[i] = labels
	}
	return out
}

func hasAny(labels []labelSet, name string) bool {
	for _, l := range labels {
		if l[name] {
			return true
		}
	}
	return false
}

func dimTaskUnderstanding(session models.Session, labels []labelSet) models.Dimension {
	score := 2
	reasons := []string{}
	if len(session.Steps) > 0 {
		score += 2
		reasons = append(reasons, "кандидат начал работу с задачей")
	}
	if hasAny(labels, "clarification") {
		score += 6
		reasons = append(reasons, "зафиксированы ограничения или критерии")
	}
	if session.FinalSubmission != nil && strings.TrimSpace(session.FinalSubmission.SelfReview) != "" {
		score += 3
		reasons = append(reasons, "есть self-review")
	}
	if hasAny(labels, "planning") {
		score += 2
		reasons = append(reasons, "видно понимание структуры работы")
	}
	return dim("Понимание задачи", score, 15, reasons, "понимание задачи выражено слабо")
}

func dimDecomposition(session models.Session, labels []labelSet) models.Dimension {
	score := 1
	reasons := []string{}
	if hasAny(labels, "planning") {
		score += 7
		reasons = append(reasons, "есть декомпозиция или план")
	}
	if len(session.Steps) >= 3 {
		score += 4
		reasons = append(reasons, "процесс разбит на несколько итераций")
	}
	if hasAny(labels, "finalization") {
		score += 2
		reasons = append(reasons, "есть финальная сборка результата")
	}
	return dim("Декомпозиция", score, 15, reasons, "задача почти не разбита на подзадачи")
}

func dimContext(session models.Session, labels []labelSet) models.Dimension {
	score := 2
	reasons := []string{}
	if hasAny(labels, "context_setting") {
		score += 7
		reasons = append(reasons, "модели задан контекст, роль или формат")
	}
	if hasLongPrompt(session.Steps) {
		score += 4
		reasons = append(reasons, "есть достаточно подробный промпт")
	}
	if strings.TrimSpace(session.Task.Instructions) != "" {
		score += 2
		reasons = append(reasons, "исходная задача сохранена в сессии")
	}
	return dim("Качество контекста", score, 15, reasons, "контекста для модели мало")
}

func dimIteration(session models.Session, labels []labelSet) models.Dimension {
	score := 0
	reasons := []string{}
	if len(session.Steps) >= 2 {
		score += 3
		reasons = append(reasons, "есть минимум две итерации")
	}
	if hasAny(labels, "iteration") {
		score += 5
		reasons = append(reasons, "кандидат улучшал или исправлял результат")
	}
	if len(session.Steps) >= 4 {
		score += 2
		reasons = append(reasons, "цепочка достаточно длинная")
	}
	return dim("Итеративность", score, 10, reasons, "нет явного улучшения результата")
}

func dimVerification(session models.Session, labels []labelSet) models.Dimension {
	score := 0
	reasons := []string{}
	if hasAny(labels, "verification") {
		score += 8
		reasons = append(reasons, "есть проверка, тесты или поиск ошибок")
	}
	if session.FinalSubmission != nil && containsAny(normalize(session.FinalSubmission.SelfReview), "провер", "ошиб", "огранич", "риск", "test", "валид") {
		score += 4
		reasons = append(reasons, "self-review содержит проверку")
	}
	if session.CodeEvaluation != nil && session.CodeEvaluation.WeightedScore > 0 {
		score += 3
		reasons = append(reasons, "кодовая часть оценена автоматически")
	}
	return dim("Контроль ошибок и галлюцинаций", score, 15, reasons, "нет заметной проверки результата")
}

func dimArtifact(session models.Session) models.Dimension {
	score := 0
	reasons := []string{}
	if session.FinalSubmission != nil && strings.TrimSpace(session.FinalSubmission.FinalAnswer) != "" {
		score += 4
		reasons = append(reasons, "финальный артефакт сдан")
	}
	if hasMeaningfulFiles(session.Files) {
		score += 5
		reasons = append(reasons, "в workspace есть рабочие файлы")
	}
	if hasReadme(session.Files) {
		score += 2
		reasons = append(reasons, "есть README/описание")
	}
	if session.CodeEvaluation != nil && session.CodeEvaluation.WeightedScore >= 65 {
		score += 4
		reasons = append(reasons, "кодовая часть выглядит полезной")
	}
	return dim("Финальный артефакт", score, 15, reasons, "итоговый артефакт слабый или отсутствует")
}

func dimEthics(session models.Session, labels []labelSet) models.Dimension {
	score := 0
	reasons := []string{}
	if hasAny(labels, "ethics") {
		score += 6
		reasons = append(reasons, "кандидат явно упомянул privacy/bias/ethics")
	}
	if session.FinalSubmission != nil && containsAny(normalize(session.FinalSubmission.SelfReview), "privacy", "приват", "bias", "этик", "предвз") {
		score += 4
		reasons = append(reasons, "этические ограничения есть в self-review")
	}
	return dim("Этика, privacy и bias", score, 10, reasons, "этические и privacy-ограничения не отражены")
}

func dimUtility(session models.Session) models.Dimension {
	score := 0
	reasons := []string{}
	if hasMeaningfulFiles(session.Files) {
		score += 2
		reasons = append(reasons, "результат можно использовать")
	}
	if session.CodeEvaluation != nil && session.CodeEvaluation.WeightedScore >= 70 {
		score += 2
		reasons = append(reasons, "код получил приемлемую оценку")
	}
	if session.FinalSubmission != nil && len([]rune(session.FinalSubmission.SelfReview)) > 80 {
		score += 1
		reasons = append(reasons, "есть осмысленный self-review")
	}
	return dim("Полезность результата", score, 5, reasons, "польза результата ограничена")
}

func dim(name string, score int, max int, reasons []string, fallback string) models.Dimension {
	if score > max {
		score = max
	}
	if score < 0 {
		score = 0
	}
	reason := fallback
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}
	return models.Dimension{Name: name, Score: score, MaxScore: max, Reason: reason}
}

func applyCaps(total int, session models.Session, labels []labelSet) int {
	if len(session.Steps) == 0 {
		return min(total, 25)
	}
	if len(session.Steps) == 1 {
		total = min(total, 45)
	}
	if len(session.Steps) == 2 {
		total = min(total, 60)
	}
	if !hasAny(labels, "verification") {
		total = min(total, 66)
	}
	if !hasAny(labels, "planning") {
		total = min(total, 70)
	}
	if !hasAny(labels, "context_setting") {
		total = min(total, 75)
	}
	if !hasAny(labels, "ethics") {
		total = min(total, 82)
	}
	if session.FinalSubmission == nil {
		total = min(total, 50)
	}
	if session.FinalSubmission != nil && len([]rune(session.FinalSubmission.SelfReview)) < 50 {
		total = min(total, 72)
	}
	if !hasMeaningfulFiles(session.Files) {
		total = min(total, 62)
	}
	return total
}

func buildFindings(session models.Session, labels []labelSet, dims []models.Dimension) ([]string, []string, []string) {
	strengths := []string{}
	weaknesses := []string{}
	redFlags := []string{}
	for _, d := range dims {
		if d.Score >= int(float64(d.MaxScore)*0.75) {
			strengths = append(strengths, d.Name+": "+d.Reason)
		}
		if d.Score <= int(float64(d.MaxScore)*0.4) {
			weaknesses = append(weaknesses, d.Name+": "+d.Reason)
		}
	}
	if len(session.Steps) < 3 {
		redFlags = append(redFlags, "слишком короткая цепочка промптов")
	}
	if !hasAny(labels, "verification") {
		redFlags = append(redFlags, "нет проверки результата")
	}
	if !hasAny(labels, "ethics") {
		redFlags = append(redFlags, "не учтены privacy/bias/ethics")
	}
	if !hasMeaningfulFiles(session.Files) {
		redFlags = append(redFlags, "нет полезного рабочего артефакта")
	}
	if len(strengths) == 0 {
		strengths = append(strengths, "кандидат дошёл до взаимодействия с ИИ и зафиксировал процесс")
	}
	return strengths, weaknesses, redFlags
}

func promptLabels(steps []models.PromptStep, labels []labelSet) []models.PromptLabel {
	out := make([]models.PromptLabel, 0, len(steps))
	for i, step := range steps {
		ls := make([]string, 0, len(labels[i]))
		for k := range labels[i] {
			ls = append(ls, k)
		}
		out = append(out, models.PromptLabel{StepNumber: step.Number, Labels: ls, Notes: "Промпт классифицирован по видимым признакам процесса."})
	}
	return out
}

func recommendation(total int, redFlags []string) string {
	if total >= 80 && len(redFlags) <= 1 {
		return "автоматически допустить к следующему этапу"
	}
	if total >= 65 {
		return "допустить с дополнительной проверкой слабых зон"
	}
	if total >= 45 {
		return "требуется ручная перепроверка или повторное задание"
	}
	return "не хватает evidence для прохождения автоматического этапа"
}

func summary(total int, session models.Session, labels []labelSet) string {
	return fmt.Sprintf("Кандидат получил %d/100 за AI-operating skill. В сессии %d промптов. Система учитывала постановку задачи, декомпозицию, контекст, итерации, проверку ошибок, этику и полезность итогового workspace.", total, len(session.Steps))
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func hasLongPrompt(steps []models.PromptStep) bool {
	for _, step := range steps {
		if len([]rune(step.Prompt)) > 160 {
			return true
		}
	}
	return false
}

func hasMeaningfulFiles(files []models.FileNode) bool {
	for _, f := range flattenFiles(files) {
		if isCodeFile(f.Name) && len([]rune(strings.TrimSpace(f.Content))) > 80 {
			return true
		}
	}
	return false
}

func hasReadme(files []models.FileNode) bool {
	for _, f := range flattenFiles(files) {
		if strings.EqualFold(f.Name, "README.md") && len([]rune(f.Content)) > 80 {
			return true
		}
	}
	return false
}

func flattenFiles(files []models.FileNode) []models.FileNode {
	out := []models.FileNode{}
	var walk func([]models.FileNode)
	walk = func(nodes []models.FileNode) {
		for _, n := range nodes {
			if n.Type == "file" {
				out = append(out, n)
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(files)
	return out
}

func isCodeFile(name string) bool {
	low := strings.ToLower(name)
	return strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".ts") || strings.HasSuffix(low, ".go") || strings.HasSuffix(low, ".py") || strings.HasSuffix(low, ".java") || strings.HasSuffix(low, ".cpp") || strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".css")
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
