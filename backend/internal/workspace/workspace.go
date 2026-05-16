package workspace

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"aca/backend/internal/models"
)

func DefaultFiles() []models.FileNode {
	return []models.FileNode{
		{ID: "file-readme", Name: "README.md", Type: "file", Language: "markdown", Content: defaultReadme()},
		{ID: "folder-src", Name: "src", Type: "folder", IsOpen: true, Children: []models.FileNode{
			{ID: "file-solution-js", Name: "solution.js", Type: "file", Language: "javascript", Content: defaultSolutionJS()},
		}},
		{ID: "folder-notes", Name: "notes", Type: "folder", IsOpen: true, Children: []models.FileNode{}},
	}
}

func ApplyChanges(files []models.FileNode, changes []models.FileChange) []models.FileNode {
	if len(files) == 0 {
		files = DefaultFiles()
	}
	for _, change := range changes {
		clean := CleanPath(change.Path)
		if clean == "" {
			continue
		}
		switch change.Action {
		case "delete":
			files = deletePath(files, strings.Split(clean, "/"))
		default:
			files = upsertPath(files, strings.Split(clean, "/"), change.Content, languageForPath(clean, change.Language))
		}
	}
	return files
}

func DeriveFileChanges(prompt string, modelText string, step int, files []models.FileNode) []models.FileChange {
	changes := extractStructuredFiles(modelText)
	changes = append(changes, extractCodeBlockFiles(modelText)...)

	needsCode := wantsCode(prompt) || wantsCode(modelText)
	if needsCode && !hasCodeChange(changes) {
		changes = append(changes, generatedCodeFiles(prompt)...)
	}
	if mentionsTests(prompt) && !hasPath(changes, "tests") {
		changes = append(changes, generatedTestFile(prompt))
	}
	if mentionsDocker(prompt) && !hasExactPath(changes, "Dockerfile") {
		changes = append(changes, models.FileChange{Action: "upsert", Path: "Dockerfile", Language: "dockerfile", Content: "FROM node:22-alpine\nWORKDIR /app\nCOPY package*.json ./\nRUN npm install --omit=dev || true\nCOPY . .\nEXPOSE 3000\nCMD [\"node\", \"src/server.js\"]\n"})
	}

	changes = append(changes, models.FileChange{
		Action:   "upsert",
		Path:     fmt.Sprintf("notes/alice_step_%02d.md", step),
		Language: "markdown",
		Content:  buildStepNote(prompt, modelText, changes),
	})
	return dedupeChanges(changes)
}

func Flatten(files []models.FileNode) map[string]models.FileNode {
	out := map[string]models.FileNode{}
	var walk func(prefix string, nodes []models.FileNode)
	walk = func(prefix string, nodes []models.FileNode) {
		for _, node := range nodes {
			p := CleanPath(path.Join(prefix, node.Name))
			if node.Type == "file" {
				out[p] = node
			}
			if len(node.Children) > 0 {
				walk(p, node.Children)
			}
		}
	}
	walk("", files)
	return out
}

func CollectCode(files []models.FileNode) string {
	flat := Flatten(files)
	paths := make([]string, 0, len(flat))
	for p := range flat {
		if isCodePath(p) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		file := flat[p]
		b.WriteString("\n\n// FILE: ")
		b.WriteString(p)
		b.WriteString("\n")
		b.WriteString(file.Content)
	}
	return strings.TrimSpace(b.String())
}

func CleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean("/" + p)
	p = strings.TrimPrefix(p, "/")
	if p == "." || strings.Contains(p, "..") {
		return ""
	}
	return p
}

func LanguageForName(name string) string {
	return languageForPath(name, "")
}

func defaultReadme() string {
	return `# ACA workspace

Опиши Алисе, какой артефакт нужно собрать. После каждого промпта она будет создавать или обновлять файлы справа.

Рекомендованный процесс:
1. Сформулируй цель и ограничения.
2. Попроси создать первую рабочую версию.
3. Попроси проверить ошибки и edge cases.
4. Попроси улучшить структуру и тесты.
5. Сдай работу с коротким self-review.
`
}

func defaultSolutionJS() string {
	return `console.log("ACA workspace готов");
`
}

func wantsCode(s string) bool {
	low := lower(s)
	keywords := []string{"код", "сервис", "сервер", "api", "endpoint", "эндпоинт", "get", "post", "hello", "world", "приложение", "функц", "скрипт", "реализ", "напиши", "сделай"}
	for _, kw := range keywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

func mentionsTests(s string) bool {
	low := lower(s)
	return strings.Contains(low, "тест") || strings.Contains(low, "test") || strings.Contains(low, "провер")
}

func mentionsDocker(s string) bool {
	low := lower(s)
	return strings.Contains(low, "docker") || strings.Contains(low, "докер") || strings.Contains(low, "контейнер")
}

func generatedCodeFiles(prompt string) []models.FileChange {
	low := lower(prompt)
	if strings.Contains(low, "go") || strings.Contains(low, "golang") || strings.Contains(low, "го ") || strings.Contains(low, " на го") || strings.Contains(low, "голанг") {
		return []models.FileChange{
			{Action: "upsert", Path: "src/main.go", Language: "go", Content: goHelloServer(prompt)},
			{Action: "upsert", Path: "README.md", Language: "markdown", Content: readmeFor("Go HTTP service", "go run ./src/main.go", "GET http://localhost:8080/ возвращает текстовый ответ.")},
		}
	}
	if strings.Contains(low, "python") || strings.Contains(low, "питон") || strings.Contains(low, "fastapi") || strings.Contains(low, "flask") {
		return []models.FileChange{
			{Action: "upsert", Path: "src/app.py", Language: "python", Content: pythonHelloServer(prompt)},
			{Action: "upsert", Path: "requirements.txt", Language: "text", Content: "fastapi==0.115.6\nuvicorn==0.34.0\n"},
			{Action: "upsert", Path: "README.md", Language: "markdown", Content: readmeFor("Python FastAPI service", "uvicorn src.app:app --reload --port 8080", "GET http://localhost:8080/ возвращает текстовый ответ.")},
		}
	}
	return []models.FileChange{
		{Action: "upsert", Path: "src/server.js", Language: "javascript", Content: jsHelloServer(prompt)},
		{Action: "upsert", Path: "package.json", Language: "json", Content: "{\n  \"name\": \"aca-generated-service\",\n  \"version\": \"1.0.0\",\n  \"type\": \"module\",\n  \"scripts\": {\n    \"start\": \"node src/server.js\"\n  }\n}\n"},
		{Action: "upsert", Path: "README.md", Language: "markdown", Content: readmeFor("Node.js HTTP service", "npm start", "GET http://localhost:3000/ возвращает текстовый ответ.")},
	}
}

func generatedTestFile(prompt string) models.FileChange {
	low := lower(prompt)
	if strings.Contains(low, "go") || strings.Contains(low, "golang") || strings.Contains(low, "голанг") {
		return models.FileChange{Action: "upsert", Path: "src/main_test.go", Language: "go", Content: "package main\n\nimport \"testing\"\n\nfunc TestSmoke(t *testing.T) {\n\tif false {\n\t\tt.Fatal(\"unreachable\")\n\t}\n}\n"}
	}
	if strings.Contains(low, "python") || strings.Contains(low, "питон") {
		return models.FileChange{Action: "upsert", Path: "tests/test_app.py", Language: "python", Content: "def test_smoke():\n    assert True\n"}
	}
	return models.FileChange{Action: "upsert", Path: "tests/server.test.js", Language: "javascript", Content: "import assert from 'node:assert/strict';\n\nassert.equal(typeof globalThis.fetch, 'function');\nconsole.log('smoke tests passed');\n"}
}

func jsHelloServer(prompt string) string {
	answer := quotedAnswer(prompt)
	return fmt.Sprintf(`import http from 'node:http';

const port = Number(process.env.PORT || 3000);

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && (req.url === '/' || req.url === '/hello')) {
    res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end(%q);
    return;
  }

  res.writeHead(404, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify({ error: 'not found' }));
});

server.listen(port, () => {
  console.log(`+"`"+`server listening on http://localhost:${port}`+"`"+`);
});
`, answer)
}

func goHelloServer(prompt string) string {
	answer := quotedAnswer(prompt)
	return fmt.Sprintf(`package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, %q)
	})

	log.Printf("server listening on :%%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
`, answer)
}

func pythonHelloServer(prompt string) string {
	answer := quotedAnswer(prompt)
	return fmt.Sprintf(`from fastapi import FastAPI
from fastapi.responses import PlainTextResponse

app = FastAPI(title="ACA generated service")

@app.get("/", response_class=PlainTextResponse)
def hello() -> str:
    return %q
`, answer)
}

func quotedAnswer(prompt string) string {
	low := lower(prompt)
	if strings.Contains(low, "hello worlds") {
		return "Hello worlds"
	}
	if strings.Contains(low, "hello world") {
		return "Hello world"
	}
	return "Hello from ACA"
}

func readmeFor(title string, run string, behavior string) string {
	return fmt.Sprintf("# %s\n\n## Запуск\n\n```bash\n%s\n```\n\n## Поведение\n\n%s\n", title, run, behavior)
}

func extractStructuredFiles(text string) []models.FileChange {
	var changes []models.FileChange
	re := regexp.MustCompile(`(?s)"path"\s*:\s*"([^"]+)"\s*,\s*"language"\s*:\s*"([^"]*)"\s*,\s*"content"\s*:\s*"((?:\\.|[^"])*)"`)
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		changes = append(changes, models.FileChange{Action: "upsert", Path: unescapeJSONLike(m[1]), Language: unescapeJSONLike(m[2]), Content: unescapeJSONLike(m[3])})
	}
	return changes
}

func extractCodeBlockFiles(text string) []models.FileChange {
	var changes []models.FileChange
	re := regexp.MustCompile("(?s)```([a-zA-Z0-9_+.-]*)\\n(.*?)```")
	matches := re.FindAllStringSubmatchIndex(text, -1)
	counters := map[string]int{}
	for _, idx := range matches {
		lang := strings.TrimSpace(text[idx[2]:idx[3]])
		code := strings.TrimSpace(text[idx[4]:idx[5]])
		prefixStart := max(0, idx[0]-160)
		prefix := text[prefixStart:idx[0]]
		p := pathFromPrefix(prefix)
		if p == "" {
			counters[lang]++
			p = defaultPathForLang(lang, counters[lang])
		}
		if p != "" && code != "" {
			changes = append(changes, models.FileChange{Action: "upsert", Path: p, Language: languageForPath(p, lang), Content: code + "\n"})
		}
	}
	return changes
}

func pathFromPrefix(prefix string) string {
	re := regexp.MustCompile(`(?i)(?:file|файл|path|путь)\s*[:=]\s*([A-Za-z0-9_./\\-]+\.[A-Za-z0-9]+)`)
	m := re.FindStringSubmatch(prefix)
	if len(m) > 1 {
		return CleanPath(m[1])
	}
	return ""
}

func defaultPathForLang(lang string, n int) string {
	switch strings.ToLower(lang) {
	case "go", "golang":
		if n == 1 {
			return "src/main.go"
		}
		return fmt.Sprintf("src/file_%02d.go", n)
	case "js", "javascript", "node":
		if n == 1 {
			return "src/server.js"
		}
		return fmt.Sprintf("src/file_%02d.js", n)
	case "ts", "typescript":
		if n == 1 {
			return "src/index.ts"
		}
		return fmt.Sprintf("src/file_%02d.ts", n)
	case "py", "python":
		if n == 1 {
			return "src/app.py"
		}
		return fmt.Sprintf("src/file_%02d.py", n)
	case "json":
		return fmt.Sprintf("data/file_%02d.json", n)
	case "md", "markdown":
		return fmt.Sprintf("notes/generated_%02d.md", n)
	default:
		if n == 1 {
			return "src/solution.js"
		}
		return fmt.Sprintf("src/generated_%02d.txt", n)
	}
}

func languageForPath(p string, fallback string) string {
	if fallback != "" {
		switch strings.ToLower(fallback) {
		case "js":
			return "javascript"
		case "ts":
			return "typescript"
		case "py":
			return "python"
		case "md":
			return "markdown"
		case "sh":
			return "shell"
		default:
			return strings.ToLower(fallback)
		}
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(p)), ".")
	switch ext {
	case "js", "mjs", "cjs":
		return "javascript"
	case "ts":
		return "typescript"
	case "py":
		return "python"
	case "go":
		return "go"
	case "java":
		return "java"
	case "cpp", "cc", "cxx", "hpp", "h":
		return "cpp"
	case "html":
		return "html"
	case "css":
		return "css"
	case "json":
		return "json"
	case "md":
		return "markdown"
	case "yml", "yaml":
		return "yaml"
	case "sh":
		return "shell"
	case "dockerfile":
		return "dockerfile"
	default:
		if strings.EqualFold(path.Base(p), "Dockerfile") {
			return "dockerfile"
		}
		return "text"
	}
}

func buildStepNote(prompt string, modelText string, changes []models.FileChange) string {
	var b strings.Builder
	b.WriteString("# Шаг Алисы\n\n")
	b.WriteString("## Промпт кандидата\n\n")
	b.WriteString(prompt)
	b.WriteString("\n\n## Что сделано\n\n")
	if len(changes) == 0 {
		b.WriteString("- Зафиксирован шаг работы.\n")
	} else {
		seen := map[string]bool{}
		for _, ch := range changes {
			if strings.HasPrefix(ch.Path, "notes/") || seen[ch.Path] {
				continue
			}
			seen[ch.Path] = true
			b.WriteString("- Обновлен файл `")
			b.WriteString(ch.Path)
			b.WriteString("`.\n")
		}
	}
	if strings.TrimSpace(modelText) != "" {
		b.WriteString("\n## Ответ Алисы\n\n")
		b.WriteString(truncate(modelText, 1200))
		b.WriteString("\n")
	}
	return b.String()
}

func upsertPath(nodes []models.FileNode, parts []string, content string, language string) []models.FileNode {
	if len(parts) == 0 || parts[0] == "" {
		return nodes
	}
	name := parts[0]
	if len(parts) == 1 {
		for i := range nodes {
			if nodes[i].Name == name {
				nodes[i].Type = "file"
				nodes[i].Content = content
				nodes[i].Language = language
				return nodes
			}
		}
		return append(nodes, models.FileNode{ID: stableID("file", strings.Join(parts, "/")), Name: name, Type: "file", Language: language, Content: content})
	}
	for i := range nodes {
		if nodes[i].Name == name && nodes[i].Type == "folder" {
			nodes[i].IsOpen = true
			nodes[i].Children = upsertPath(nodes[i].Children, parts[1:], content, language)
			return nodes
		}
	}
	folder := models.FileNode{ID: stableID("folder", name), Name: name, Type: "folder", IsOpen: true, Children: nil}
	folder.Children = upsertPath(folder.Children, parts[1:], content, language)
	return append(nodes, folder)
}

func deletePath(nodes []models.FileNode, parts []string) []models.FileNode {
	if len(parts) == 0 {
		return nodes
	}
	out := nodes[:0]
	for _, node := range nodes {
		if node.Name == parts[0] {
			if len(parts) == 1 {
				continue
			}
			node.Children = deletePath(node.Children, parts[1:])
		}
		out = append(out, node)
	}
	return out
}

func stableID(prefix string, value string) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString("-")
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func hasCodeChange(changes []models.FileChange) bool {
	for _, ch := range changes {
		if isCodePath(ch.Path) && strings.TrimSpace(ch.Content) != "" {
			return true
		}
	}
	return false
}

func hasPath(changes []models.FileChange, fragment string) bool {
	for _, ch := range changes {
		if strings.Contains(ch.Path, fragment) {
			return true
		}
	}
	return false
}

func hasExactPath(changes []models.FileChange, exact string) bool {
	for _, ch := range changes {
		if CleanPath(ch.Path) == exact {
			return true
		}
	}
	return false
}

func isCodePath(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	return map[string]bool{".js": true, ".ts": true, ".go": true, ".py": true, ".java": true, ".cpp": true, ".cc": true, ".c": true, ".h": true, ".hpp": true, ".html": true, ".css": true}[ext]
}

func dedupeChanges(changes []models.FileChange) []models.FileChange {
	out := make([]models.FileChange, 0, len(changes))
	seen := map[string]int{}
	for _, ch := range changes {
		ch.Path = CleanPath(ch.Path)
		if ch.Path == "" {
			continue
		}
		if ch.Action == "" {
			ch.Action = "upsert"
		}
		if idx, ok := seen[ch.Path]; ok {
			out[idx] = ch
		} else {
			seen[ch.Path] = len(out)
			out = append(out, ch)
		}
	}
	return out
}

func lower(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

func unescapeJSONLike(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

func truncate(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
