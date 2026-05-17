<script lang="ts">
  import { onMount } from 'svelte';
  import Editor from 'editor-for-svelte';
  import 'highlight.js/styles/github-dark.css';
  import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "$lib/components/ui/card";
  import { Tabs, TabsContent, TabsList, TabsTrigger } from "$lib/components/ui/tabs";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Badge } from "$lib/components/ui/badge";
  import { ScrollArea } from "$lib/components/ui/scroll-area";
  import { Separator } from "$lib/components/ui/separator";
  import { Switch } from "$lib/components/ui/switch";
  import { Avatar, AvatarFallback } from "$lib/components/ui/avatar";
  import * as Select from "$lib/components/ui/select";
  import {
    Sun, Moon, Play, Trash2, Send, RotateCcw,
    FileText, BarChart3, GripVertical,
    FolderOpen, FolderPlus, FilePlus,
    Folder, ChevronRight, ChevronDown, X,
    Plus, Save, Copy, Check, Eye, User, Download
  } from 'lucide-svelte';

  let {
    code = "",
    language = "javascript",
    defaultLeftWidth = 50
  }: {
    code?: string;
    language?: string;
    defaultLeftWidth?: number;
  } = $props();

  interface FileNode {
    id: string;
    name: string;
    type: 'file' | 'folder';
    language?: string;
    content?: string;
    children?: FileNode[];
    isOpen?: boolean;
  }

  interface FileChange {
    action: string;
    path: string;
    language?: string;
    content?: string;
  }

  interface PromptStep {
    id: string;
    number: number;
    prompt: string;
    model_output: string;
    model_provider: string;
    model_name: string;
    duration_ms: number;
    file_changes?: FileChange[];
    created_at: string;
  }

  interface Dimension {
    name: string;
    score: number;
    max_score: number;
    reason: string;
  }

  interface ReviewReport {
    total_score: number;
    dimensions: Dimension[];
    strengths: string[];
    weaknesses: string[];
    red_flags: string[];
    recommendation: string;
    summary: string;
    next_interview_questions: string[];
    practical_tasks: string[];
    ethics_privacy_notes: string[];
    generated_by: string;
  }

  interface SessionData {
    id: string;
    candidate_token?: string;
    reviewer_token?: string;
    candidate_url?: string;
    reviewer_url?: string;
    task: {
      title: string;
      instructions: string;
      role: string;
      timebox_minutes: number;
      expected_deliverables: string[];
      allow_code: boolean;
      language: string;
    };
    files: FileNode[];
    steps: PromptStep[];
    final_submission?: unknown;
    code_evaluation?: unknown;
    review?: ReviewReport;
    updated_at: string;
  }

  type Role = 'candidate' | 'reviewer';

  const API_BASE = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

  let activeTab = $state('description');
  let leftWidth = $state(defaultLeftWidth);
  let isResizing = $state(false);
  let isDark = $state(false);
  let role = $state<Role>('candidate');
  let runsCount = $state(0);
  let errorsCount = $state(0);
  let startTime = $state(Date.now());
  let elapsed = $state('00:00');
  let selectedModel = $state('alice');
  let isAiTyping = $state(false);
  let showFileExplorer = $state(true);
  let activeFileId = $state('file-readme');
  let newItemName = $state('');
  let isCreating = $state(false);
  let createType = $state<'file' | 'folder'>('file');
  let parentFolderId = $state<string | null>(null);
  let aiInput = $state('');
  let saveStatus = $state('не сохранено');
  let isLoadingSession = $state(true);
  let sessionId = $state('');
  let token = $state('');
  let candidateURL = $state('');
  let reviewerURL = $state('');
  let copyStatus = $state('');
  let review = $state<ReviewReport | null>(null);
  let submitStatus = $state('');
  let pollHandle: number | undefined;
  let saveHandle: number | undefined;

  let aiMessages = $state<Array<{role: string, content: string}>>([
    {role: 'assistant', content: 'Новая сессия создаётся. После старта пиши промпт Алисе, а я буду создавать файлы в workspace.'}
  ]);

  let files = $state<FileNode[]>([
    { id: 'file-readme', name: 'README.md', type: 'file', language: 'markdown', content: '# ACA workspace\n\nОпиши Алисе, что нужно собрать.\n' },
    { id: 'folder-src', name: 'src', type: 'folder', isOpen: true, children: [
      { id: 'file-solution-js', name: 'solution.js', type: 'file', language: 'javascript', content: code || 'console.log("ACA workspace готов");\n' }
    ]},
    { id: 'folder-notes', name: 'notes', type: 'folder', isOpen: true, children: [] }
  ]);

  function asteroidLogoClass() {
    return 'bg-gradient-to-br from-fuchsia-500 to-purple-600 text-white text-sm font-semibold';
  }

  function isReviewer() {
    return role === 'reviewer';
  }

  function getActiveFile(): FileNode | null {
    const find = (nodes: FileNode[]): FileNode | null => {
      for (const node of nodes) {
        if (node.id === activeFileId && node.type === 'file') return node;
        if (node.children) {
          const found = find(node.children);
          if (found) return found;
        }
      }
      return null;
    };
    return find(files);
  }

  function getEditorCode(): string {
    return getActiveFile()?.content || '';
  }

  function getEditorLanguage(): string {
    return getActiveFile()?.language || language || 'text';
  }

  function flattenFiles(nodes: FileNode[], prefix = ''): Array<{path: string; node: FileNode}> {
    const out: Array<{path: string; node: FileNode}> = [];
    for (const node of nodes) {
      const current = prefix ? `${prefix}/${node.name}` : node.name;
      if (node.type === 'file') out.push({ path: current, node });
      if (node.children) out.push(...flattenFiles(node.children, current));
    }
    return out;
  }

  function countFiles(nodes: FileNode[]): number {
    return flattenFiles(nodes).length;
  }

  function firstFileId(nodes: FileNode[]): string {
    for (const node of nodes) {
      if (node.type === 'file') return node.id;
      if (node.children) {
        const id = firstFileId(node.children);
        if (id) return id;
      }
    }
    return 'file-readme';
  }

  function toggleFolder(id: string) {
    files = mapNodes(files, (node) => node.id === id ? { ...node, isOpen: !node.isOpen } : node);
    scheduleSave();
  }

  function selectFile(id: string) {
    activeFileId = id;
  }

  function startCreateItem(type: 'file' | 'folder', parentId: string | null = null) {
    if (isReviewer()) return;
    isCreating = true;
    createType = type;
    parentFolderId = parentId;
    newItemName = '';
  }

  function createItem() {
    if (!newItemName.trim() || isReviewer()) return;
    const name = newItemName.trim();
    const newItem: FileNode = createType === 'file'
      ? { id: `file-${Date.now()}`, name, type: 'file', language: getLanguageFromExtension(name), content: '' }
      : { id: `folder-${Date.now()}`, name, type: 'folder', isOpen: true, children: [] };

    if (parentFolderId) {
      files = mapNodes(files, (node) => node.id === parentFolderId
        ? { ...node, isOpen: true, children: [...(node.children || []), newItem] }
        : node
      );
    } else {
      files = [...files, newItem];
    }

    isCreating = false;
    newItemName = '';
    if (createType === 'file') selectFile(newItem.id);
    scheduleSave();
  }

  function deleteFile(id: string) {
    if (isReviewer()) return;
    files = removeNode(files, id);
    if (activeFileId === id) activeFileId = firstFileId(files);
    scheduleSave();
  }

  function getLanguageFromExtension(filename: string): string {
    const ext = filename.split('.').pop()?.toLowerCase();
    const map: Record<string, string> = {
      js: 'javascript', mjs: 'javascript', ts: 'typescript', py: 'python', go: 'go', html: 'html', css: 'css', json: 'json', md: 'markdown', jsx: 'jsx', tsx: 'tsx', yml: 'yaml', yaml: 'yaml', sh: 'shell', cpp: 'cpp', cc: 'cpp', h: 'cpp', hpp: 'cpp'
    };
    if (filename === 'Dockerfile') return 'dockerfile';
    return map[ext || ''] || 'text';
  }

  function updateFileContent(content: string) {
    if (isReviewer()) return;
    files = mapNodes(files, (node) => node.id === activeFileId ? { ...node, content } : node);
    scheduleSave();
  }

  function mapNodes(nodes: FileNode[], fn: (node: FileNode) => FileNode): FileNode[] {
    return nodes.map((node) => {
      const mapped = fn(node);
      if (mapped.children) return { ...mapped, children: mapNodes(mapped.children, fn) };
      return mapped;
    });
  }

  function removeNode(nodes: FileNode[], id: string): FileNode[] {
    return nodes
      .filter((node) => node.id !== id)
      .map((node) => ({ ...node, children: node.children ? removeNode(node.children, id) : undefined }));
  }

  function storageKey() {
    return sessionId ? `aca.workspace.${sessionId}` : 'aca.workspace.new';
  }

  function saveLocal() {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(storageKey(), JSON.stringify(files));
  }

  function loadLocal(): FileNode[] | null {
    if (typeof localStorage === 'undefined' || !sessionId) return null;
    const raw = localStorage.getItem(storageKey());
    if (!raw) return null;
    try {
      const parsed = JSON.parse(raw);
      return Array.isArray(parsed) ? parsed : null;
    } catch {
      return null;
    }
  }

  function scheduleSave() {
    saveLocal();
    saveStatus = 'сохранение...';
    if (saveHandle) window.clearTimeout(saveHandle);
    saveHandle = window.setTimeout(() => saveWorkspace(), 700);
  }

  async function saveWorkspace() {
    if (!sessionId || !token || isReviewer()) return;
    try {
      await fetch(`${API_BASE}/api/sessions/${sessionId}/files?token=${token}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ files })
      });
      saveStatus = 'сохранено';
      saveLocal();
    } catch {
      saveStatus = 'локально сохранено';
    }
  }

  function messageFromRaw(raw: string): string {
    if (!raw) return 'Алиса обновила workspace.';
    try {
      const start = raw.indexOf('{');
      const end = raw.lastIndexOf('}');
      if (start >= 0 && end > start) {
        const parsed = JSON.parse(raw.slice(start, end + 1));
        if (parsed.message) return parsed.message;
      }
    } catch {}
    return raw.length > 700 ? raw.slice(0, 700) + '…' : raw;
  }

  function rebuildMessages(session: SessionData) {
    const messages = [{role: 'assistant', content: isReviewer() ? 'Режим ревьюера: сессия открыта для наблюдения.' : 'Новая сессия создана. Пиши промпт Алисе, а я буду создавать файлы в workspace.'}];
    for (const step of session.steps || []) {
      messages.push({ role: 'user', content: step.prompt });
      messages.push({ role: 'assistant', content: messageFromRaw(step.model_output) });
    }
    aiMessages = messages;
  }

  async function createSession() {
    const resp = await fetch(`${API_BASE}/api/sessions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });
    const session: SessionData = await resp.json();
    sessionId = session.id;
    token = session.candidate_token || '';
    role = 'candidate';
    candidateURL = session.candidate_url || '';
    reviewerURL = session.reviewer_url || '';
    localStorage.setItem('aca.lastSession', JSON.stringify({ sessionId, token, role }));
    history.replaceState(null, '', `/?session=${sessionId}&token=${token}&role=candidate`);
    applySession(session, true);
  }

  async function loadSession() {
    if (!sessionId || !token) return;
    const resp = await fetch(`${API_BASE}/api/sessions/${sessionId}?token=${token}`);
    if (!resp.ok) throw new Error('session load failed');
    const session: SessionData = await resp.json();
    applySession(session, false);
  }

  function applySession(session: SessionData, initial: boolean) {
    candidateURL = session.candidate_url || candidateURL;
    reviewerURL = session.reviewer_url || reviewerURL;
    review = session.review || null;
    const local = !isReviewer() && initial ? loadLocal() : null;
    files = local || session.files || files;
    if (!getActiveFile()) activeFileId = firstFileId(files);
    rebuildMessages(session);
    saveLocal();
    saveStatus = 'сохранено';
    if (review && activeTab !== 'ai') activeTab = isReviewer() ? 'stats' : activeTab;
  }

  async function handleAiSend() {
    if (!aiInput.trim() || isAiTyping || isReviewer()) return;
    const prompt = aiInput.trim();
    aiMessages = [...aiMessages, { role: 'user', content: prompt }];
    aiInput = '';
    isAiTyping = true;
    await saveWorkspace();
    try {
      const resp = await fetch(`${API_BASE}/api/sessions/${sessionId}/prompt?token=${token}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt, files, selected_model: selectedModel, active_file_id: activeFileId })
      });
      if (!resp.ok) throw new Error(await resp.text());
      const data = await resp.json();
      aiMessages = [...aiMessages, { role: 'assistant', content: data.message || 'Алиса обновила workspace.' }];
      if (data.session?.files) files = data.session.files;
      if (data.session?.review) review = data.session.review;
      const next = pickFileFromChanges(data.file_changes || []);
      if (next) activeFileId = next;
      saveLocal();
      saveStatus = 'сохранено';
    } catch (e: any) {
      aiMessages = [...aiMessages, { role: 'assistant', content: `Не смогла выполнить запрос: ${e?.message || 'ошибка сети'}` }];
    } finally {
      isAiTyping = false;
    }
  }

  function pickFileFromChanges(changes: FileChange[]): string | null {
    const preferred = changes.find((change) => !change.path.startsWith('notes/') && isCodeLike(change.path)) || changes.find((change) => !change.path.startsWith('notes/'));
    if (!preferred) return null;
    const path = preferred.path;
    const item = flattenFiles(files).find((entry) => entry.path === path);
    return item?.node.id || null;
  }

  function isCodeLike(path: string) {
    return /\.(js|ts|go|py|java|cpp|cc|html|css)$/i.test(path);
  }

  async function submitWork() {
    if (isReviewer()) return;
    await saveWorkspace();
    submitStatus = 'анализ...';
    const finalAnswer = getReadme() || getEditorCode();
    const codeText = flattenFiles(files)
      .filter((f) => isCodeLike(f.path))
      .map((f) => `// FILE: ${f.path}\n${f.node.content || ''}`)
      .join('\n\n');
    const selfReview = window.prompt('Короткий self-review: что проверил, какие ограничения есть, что бы улучшил?', 'Проверил базовую структуру, запуск и ограничения. Следующий шаг — добавить тесты и edge cases.') || '';
    try {
      const resp = await fetch(`${API_BASE}/api/sessions/${sessionId}/submit?token=${token}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ final_answer: finalAnswer, code: codeText, self_review: selfReview, files })
      });
      const session: SessionData = await resp.json();
      applySession(session, false);
      submitStatus = 'сдано';
      activeTab = 'stats';
    } catch {
      submitStatus = 'ошибка сдачи';
    }
  }

  function getReadme() {
    return flattenFiles(files).find((f) => f.path === 'README.md')?.node.content || '';
  }

  async function copyText(text: string, label: string) {
    try {
      await navigator.clipboard.writeText(text);
      copyStatus = `${label} скопирована`;
      setTimeout(() => copyStatus = '', 1500);
    } catch {
      copyStatus = 'не удалось скопировать';
    }
  }

  function clearChat() {
    if (isReviewer()) return;
    aiMessages = [{ role: 'assistant', content: 'Чат очищен локально. Файлы и история сессии на сервере сохранены.' }];
  }

  function handleRun() {
    try {
      runsCount++;
      const editorCode = getEditorCode();
      const lang = getEditorLanguage();
      if (lang === 'javascript') {
        const originalLog = console.log;
        const buffer: string[] = [];
        console.log = (...args) => {
          buffer.push(args.map(a => (typeof a === 'object' ? JSON.stringify(a, null, 2) : String(a))).join(' '));
          originalLog(...args);
        };
        const result = eval(editorCode);
        console.log = originalLog;
        if (result !== undefined) buffer.push(`→ ${JSON.stringify(result, null, 2)}`);
      }
    } catch {
      errorsCount++;
    }
  }

  function startResizing() {
    isResizing = true;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  }

  function stopResizing() {
    isResizing = false;
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  }

  function onResize(e: MouseEvent) {
    if (!isResizing) return;
    const container = document.getElementById('split-container');
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const newLeftWidth = ((e.clientX - rect.left) / rect.width) * 100;
    leftWidth = Math.min(85, Math.max(15, newLeftWidth));
  }

  function resetLayout() {
    leftWidth = 50;
  }

  function downloadWorkspace() {
    const payload = JSON.stringify({ session_id: sessionId, files }, null, 2);
    const blob = new Blob([payload], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `aca-workspace-${sessionId || 'local'}.json`;
    a.click();
    URL.revokeObjectURL(url);
  }

  function initPolling() {
    if (pollHandle) window.clearInterval(pollHandle);
    if (isReviewer()) {
      pollHandle = window.setInterval(() => loadSession().catch(() => {}), 2000);
    }
  }

  $effect(() => {
    if (isDark) document.documentElement.classList.add('dark');
    else document.documentElement.classList.remove('dark');
  });

  $effect(() => {
    startTime = Date.now();
    const interval = setInterval(() => {
      const diff = Date.now() - startTime;
      const mins = Math.floor(diff / 60000);
      const secs = Math.floor((diff % 60000) / 1000);
      elapsed = `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }, 1000);
    return () => clearInterval(interval);
  });

  $effect(() => {
    window.addEventListener('mousemove', onResize);
    window.addEventListener('mouseup', stopResizing);
    return () => {
      window.removeEventListener('mousemove', onResize);
      window.removeEventListener('mouseup', stopResizing);
    };
  });

  onMount(async () => {
    const params = new URLSearchParams(location.search);
    const urlSession = params.get('session');
    const urlToken = params.get('token');
    const urlRole = params.get('role') as Role | null;
    if (urlSession && urlToken) {
      sessionId = urlSession;
      token = urlToken;
      role = urlRole === 'reviewer' ? 'reviewer' : 'candidate';
      await loadSession().catch(() => createSession());
    } else {
      await createSession();
    }
    isLoadingSession = false;
    initPolling();
  });
</script>

<style>
  :global(.dark .cm-editor) { background-color: #0a0a0a !important; }
  :global(.dark .cm-editor .cm-gutters) { background-color: #0a0a0a !important; border-right: 1px solid #1a1a1a !important; color: #444 !important; }
  :global(.dark .cm-editor .cm-activeLine) { background-color: #111111 !important; }
  :global(.dark .cm-editor .cm-activeLineGutter) { background-color: #0a0a0a !important; color: #555 !important; }
  :global(.dark .cm-editor .cm-cursor) { border-left-color: #ffffff !important; border-left-width: 2px !important; margin-left: -1px !important; }
  :global(.dark .cm-editor.cm-focused .cm-cursor) { border-left-color: #00ff88 !important; border-left-width: 2px !important; margin-left: -1px !important; animation: none !important; }
  :global(.dark .cm-editor .cm-selectionBackground) { background-color: #1a3a5c !important; }
  :global(.dark .cm-editor .cm-line) { color: #e0e0e0 !important; }
  .custom-scrollbar::-webkit-scrollbar { width: 6px; }
  .custom-scrollbar::-webkit-scrollbar-track { background: transparent; }
  .custom-scrollbar::-webkit-scrollbar-thumb { background: hsl(var(--border)); border-radius: 3px; }
  .typing-dot { animation: typingBounce 1.4s infinite ease-in-out both; }
  .typing-dot:nth-child(1) { animation-delay: -0.32s; }
  .typing-dot:nth-child(2) { animation-delay: -0.16s; }
  @keyframes typingBounce { 0%, 80%, 100% { transform: scale(0); } 40% { transform: scale(1); } }
</style>

<div class="min-h-screen bg-background text-foreground">
  <div class="flex items-center justify-between px-6 py-3 border-b">
    <div class="flex items-center gap-3">
      <div class="text-lg font-black tracking-tight">ACA</div>
      <Badge variant="outline" class="text-xs gap-1">
        <FileText class="w-3 h-3" />
        {getEditorLanguage()}
      </Badge>
      <Badge variant="secondary" class="text-xs">
        {getEditorCode().split('\n').length} lines
      </Badge>
      {#if sessionId}
        <Badge variant="outline" class="text-xs">session {sessionId.slice(0, 8)}</Badge>
      {/if}
    </div>
    <div class="flex items-center gap-3">
      <Button variant={role === 'candidate' ? 'default' : 'ghost'} size="sm" class="gap-1 text-xs" onclick={() => { role = 'candidate'; if (candidateURL) location.href = candidateURL; }}>
        <User class="w-3 h-3" /> Кандидат
      </Button>
      <Button variant={role === 'reviewer' ? 'default' : 'ghost'} size="sm" class="gap-1 text-xs" onclick={() => { role = 'reviewer'; if (reviewerURL) location.href = reviewerURL; }}>
        <Eye class="w-3 h-3" /> Ревьюер
      </Button>
      <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={() => copyText(candidateURL, 'ссылка кандидата')} disabled={!candidateURL}>
        <Copy class="w-3 h-3" /> Кандидат link
      </Button>
      <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={() => copyText(reviewerURL, 'ссылка ревьюера')} disabled={!reviewerURL}>
        <Copy class="w-3 h-3" /> Reviewer link
      </Button>
      {#if copyStatus}<span class="text-xs text-muted-foreground">{copyStatus}</span>{/if}
      <Separator orientation="vertical" class="h-6" />
      <div class="flex items-center gap-2">
        <Switch bind:checked={isDark} id="theme-mode" />
        <label for="theme-mode" class="text-sm cursor-pointer select-none">dark</label>
      </div>
      <Button variant="ghost" size="icon" onclick={() => isDark = !isDark} title={isDark ? 'Светлая тема' : 'Темная тема'}>
        {#if isDark}<Sun class="w-5 h-5" />{:else}<Moon class="w-5 h-5" />{/if}
      </Button>
    </div>
  </div>

  <div id="split-container" class="flex h-[calc(100vh-4rem)] w-full gap-0 p-6">
    <div class="overflow-hidden transition-none pr-4" style="width: {leftWidth}%;">
      <Tabs value={activeTab} onValueChange={(v) => activeTab = v} class="h-full flex flex-col">
        <TabsList class="w-full mb-4 flex-shrink-0">
          <TabsTrigger value="description" class="flex-1 gap-2"><FileText class="w-4 h-4" />Задание</TabsTrigger>
          <TabsTrigger value="ai" class="flex-1 gap-2"><span class="text-base leading-none">☄</span> Алиса</TabsTrigger>
          {#if isReviewer() || review}
            <TabsTrigger value="stats" class="flex-1 gap-2"><BarChart3 class="w-4 h-4" />Ревью</TabsTrigger>
          {/if}
        </TabsList>

        <TabsContent value="description" class="flex-1 min-h-0">
          <ScrollArea class="h-full">
            <Card>
              <CardHeader>
                <CardTitle>{isLoadingSession ? 'Загрузка...' : 'AI Interview: проверка AI-operating skill'}</CardTitle>
                <CardDescription>Задача проверяет, как кандидат управляет ИИ, а не просто пишет один идеальный промпт.</CardDescription>
              </CardHeader>
              <CardContent class="space-y-6">
                <div>
                  <h3 class="font-semibold mb-2">Что нужно сделать</h3>
                  <p class="text-sm text-muted-foreground leading-relaxed">За 25 минут получи полезный рабочий артефакт с помощью Алисы. Можно просить Алису создавать код, README, тесты, план, проверки и улучшения. Все промпты, ответы и изменения файлов сохраняются в сессии.</p>
                </div>
                <div>
                  <h3 class="font-semibold mb-2">Что оценивается</h3>
                  <ul class="list-disc list-inside text-sm text-muted-foreground space-y-2">
                    <li>Понимание задачи и ограничений</li>
                    <li>Декомпозиция на подзадачи</li>
                    <li>Качество контекста для Алисы</li>
                    <li>Итерации и улучшение результата</li>
                    <li>Проверка ошибок, тестов и галлюцинаций</li>
                    <li>Финальный workspace и self-review</li>
                  </ul>
                </div>
                <div>
                  <h3 class="font-semibold mb-2">Пример хорошего процесса</h3>
                  <pre class="bg-muted p-4 rounded-md text-sm leading-relaxed whitespace-pre-wrap">1. Алиса, сначала уточни ограничения и предложи план решения.
2. Создай рабочий сервис и файлы проекта.
3. Проверь edge cases, ошибки запуска и безопасность.
4. Добавь тесты или smoke-check.
5. Сформируй финальное README и self-review.</pre>
                </div>
              </CardContent>
            </Card>
          </ScrollArea>
        </TabsContent>

        <TabsContent value="ai" class="flex-1 min-h-0 overflow-hidden">
          <Card class="h-full flex flex-col">
            <CardHeader class="flex-shrink-0 pb-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <Avatar class="h-9 w-9"><AvatarFallback class={asteroidLogoClass()}>☄</AvatarFallback></Avatar>
                  <div>
                    <CardTitle class="text-base">Алиса</CardTitle>
                    <CardDescription class="text-xs">AI-ассистент с автосозданием файлов</CardDescription>
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <Select.Root type="single" bind:value={selectedModel}>
                    <Select.Trigger class="h-9 w-[170px] text-xs">
                      <span class="flex items-center gap-2"><span>☄</span>{selectedModel === 'alice' ? 'Алиса' : 'GigaChat'}</span>
                    </Select.Trigger>
                    <Select.Content>
                      <Select.Item value="alice"><span class="flex items-center gap-2"><span>☄</span>Алиса</span></Select.Item>
                      <Select.Item value="gigachat" disabled><span class="flex items-center gap-2 opacity-60"><span>◇</span>GigaChat скоро</span></Select.Item>
                    </Select.Content>
                  </Select.Root>
                  <Button variant="ghost" size="icon" class="h-8 w-8" onclick={clearChat} title="Очистить чат" disabled={isReviewer()}><Trash2 class="w-4 h-4" /></Button>
                </div>
              </div>
            </CardHeader>
            <CardContent class="flex-1 flex flex-col min-h-0 p-0">
              <div class="flex-1 overflow-y-auto custom-scrollbar px-6" style="scroll-behavior: smooth;">
                <div class="space-y-4 py-4">
                  {#each aiMessages as message}
                    {#if message.role === 'user'}
                      <div class="flex justify-end"><div class="max-w-[80%] bg-primary text-primary-foreground rounded-2xl rounded-br-sm px-4 py-2.5 text-sm whitespace-pre-wrap">{message.content}</div></div>
                    {:else}
                      <div class="flex gap-3">
                        <Avatar class="h-8 w-8 flex-shrink-0"><AvatarFallback class={asteroidLogoClass()}>☄</AvatarFallback></Avatar>
                        <div class="max-w-[80%] bg-muted rounded-2xl rounded-bl-sm px-4 py-2.5 text-sm leading-relaxed whitespace-pre-wrap">{message.content}</div>
                      </div>
                    {/if}
                  {/each}
                  {#if isAiTyping}
                    <div class="flex gap-3">
                      <Avatar class="h-8 w-8 flex-shrink-0"><AvatarFallback class={asteroidLogoClass()}>☄</AvatarFallback></Avatar>
                      <div class="bg-muted rounded-2xl rounded-bl-sm px-4 py-3"><div class="flex gap-1.5"><div class="w-2 h-2 bg-muted-foreground/40 rounded-full typing-dot"></div><div class="w-2 h-2 bg-muted-foreground/40 rounded-full typing-dot"></div><div class="w-2 h-2 bg-muted-foreground/40 rounded-full typing-dot"></div></div></div>
                    </div>
                  {/if}
                </div>
              </div>
              <div class="flex-shrink-0 border-t p-4">
                <div class="flex gap-3">
                  <Input bind:value={aiInput} placeholder={isReviewer() ? 'Ревьюер наблюдает за сессией...' : 'Напиши промпт Алисе...'} class="flex-1" disabled={isReviewer() || isAiTyping} onkeydown={(e) => e.key === 'Enter' && !e.shiftKey && handleAiSend()} />
                  <Button onclick={handleAiSend} disabled={isAiTyping || !aiInput.trim() || isReviewer()} size="icon"><Send class="w-5 h-5" /></Button>
                </div>
                <div class="flex justify-between items-center mt-2 px-1">
                  <span class="text-xs text-muted-foreground">☄ Алиса</span>
                  <Badge variant="outline" class="text-xs">{aiMessages.length} сообщений</Badge>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {#if isReviewer() || review}
          <TabsContent value="stats" class="flex-1 min-h-0">
            <ScrollArea class="h-full">
              <div class="space-y-4">
                <Card>
                  <CardHeader>
                    <div class="flex items-center gap-2"><BarChart3 class="w-5 h-5" /><CardTitle>Автоматическое ревью</CardTitle></div>
                    <CardDescription>Ревьюер видит сессию в реальном времени по отдельной ссылке.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    {#if review}
                      <div class="space-y-5">
                        <div class="flex items-center gap-4">
                          <div class="text-5xl font-bold tabular-nums">{review.total_score}</div>
                          <div><div class="text-sm text-muted-foreground">из 100</div><div class="font-medium">{review.recommendation}</div></div>
                        </div>
                        <p class="text-sm text-muted-foreground leading-relaxed">{review.summary}</p>
                        <div class="space-y-2">
                          {#each review.dimensions as d}
                            <div class="border rounded-lg p-3">
                              <div class="flex justify-between text-sm font-medium"><span>{d.name}</span><span>{d.score}/{d.max_score}</span></div>
                              <div class="text-xs text-muted-foreground mt-1">{d.reason}</div>
                            </div>
                          {/each}
                        </div>
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
                          <div><h4 class="font-semibold mb-2">Сильные стороны</h4><ul class="list-disc list-inside text-muted-foreground space-y-1">{#each review.strengths as item}<li>{item}</li>{/each}</ul></div>
                          <div><h4 class="font-semibold mb-2">Слабые зоны</h4><ul class="list-disc list-inside text-muted-foreground space-y-1">{#each review.weaknesses as item}<li>{item}</li>{/each}</ul></div>
                          <div><h4 class="font-semibold mb-2">Red flags</h4><ul class="list-disc list-inside text-muted-foreground space-y-1">{#each review.red_flags as item}<li>{item}</li>{/each}</ul></div>
                          <div><h4 class="font-semibold mb-2">Практические задачи</h4><ul class="list-disc list-inside text-muted-foreground space-y-1">{#each review.practical_tasks as item}<li>{item}</li>{/each}</ul></div>
                        </div>
                      </div>
                    {:else}
                      <div class="text-sm text-muted-foreground">Ревью появится после сдачи работы кандидатом. В режиме ревьюера данные обновляются автоматически.</div>
                    {/if}
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader><CardTitle>Статистика сессии</CardTitle></CardHeader>
                  <CardContent>
                    <dl class="grid grid-cols-2 gap-6 text-sm">
                      <div><dt class="text-muted-foreground mb-1">Прошло времени</dt><dd class="text-3xl font-semibold tabular-nums">{elapsed}</dd></div>
                      <div><dt class="text-muted-foreground mb-1">Запуски кода</dt><dd class="text-3xl font-semibold">{runsCount}</dd></div>
                      <div><dt class="text-muted-foreground mb-1">Ошибки запуска</dt><dd class="text-3xl font-semibold">{errorsCount}</dd></div>
                      <div><dt class="text-muted-foreground mb-1">Файлов</dt><dd class="text-3xl font-semibold">{countFiles(files)}</dd></div>
                    </dl>
                  </CardContent>
                </Card>
              </div>
            </ScrollArea>
          </TabsContent>
        {/if}
      </Tabs>
    </div>

    <div class="relative group flex items-center justify-center cursor-col-resize mx-2" style="width: 4px;" onmousedown={startResizing} onmouseup={stopResizing} role="separator" aria-label="Resize panels" aria-orientation="vertical">
      <div class="absolute inset-y-0 -left-3 -right-3"></div>
      <div class="w-1 h-full bg-border group-hover:bg-primary/70 group-hover:w-1.5 transition-all duration-200 rounded-full"></div>
      <div class="absolute top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 transition-all duration-200 pointer-events-none"><div class="bg-primary text-primary-foreground rounded-full p-1.5 shadow-lg"><GripVertical class="w-3 h-3" /></div></div>
    </div>

    <div class="flex flex-col gap-5 transition-none pl-4" style="width: {100 - leftWidth}%;">
      <div class="flex justify-between items-center">
        <div class="flex items-center gap-2">
          <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={() => showFileExplorer = !showFileExplorer}><FolderOpen class="w-4 h-4" />{showFileExplorer ? 'Скрыть файлы' : 'Показать файлы'}</Button>
          <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={() => startCreateItem('file')} disabled={isReviewer()}><FilePlus class="w-4 h-4" />Новый файл</Button>
          <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={() => startCreateItem('folder')} disabled={isReviewer()}><FolderPlus class="w-4 h-4" />Новая папка</Button>
          <Button variant="ghost" size="sm" class="gap-1 text-xs" onclick={saveWorkspace} disabled={isReviewer()}><Save class="w-4 h-4" />Save</Button>
          <span class="text-xs text-muted-foreground">{saveStatus}</span>
        </div>
        <Button variant="ghost" size="sm" onclick={resetLayout} class="text-xs gap-1"><RotateCcw class="w-3 h-3" />Reset split</Button>
      </div>

      {#if isCreating}
        <div class="flex items-center gap-2 p-3 border rounded-lg bg-muted/50">
          <span class="text-xs">Создать {createType === 'file' ? 'файл' : 'папку'}:</span>
          <Input bind:value={newItemName} placeholder={createType === 'file' ? 'app.js' : 'components'} class="h-7 text-xs flex-1" onkeydown={(e) => { if (e.key === 'Enter') createItem(); if (e.key === 'Escape') { isCreating = false; newItemName = ''; } }} autofocus />
          <Button size="sm" class="h-7 text-xs gap-1" onclick={createItem}><Plus class="w-3 h-3" />Создать</Button>
          <Button variant="ghost" size="icon" class="h-7 w-7" onclick={() => { isCreating = false; newItemName = ''; }}><X class="w-3 h-3" /></Button>
        </div>
      {/if}

      {#if showFileExplorer}
        <div class="border rounded-lg overflow-hidden">
          <div class="bg-muted px-4 py-2 text-sm font-medium border-b flex justify-between items-center">
            <span class="flex items-center gap-2"><FolderOpen class="w-4 h-4" />Файлы проекта</span>
            <span class="text-xs text-muted-foreground">{countFiles(files)} файлов</span>
          </div>
          <ScrollArea class="h-56">
            <div class="p-2 space-y-1">
              {#each files as node}
                <div class="space-y-1">
                  {#if node.type === 'folder'}
                    <div class="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted cursor-pointer group" onclick={() => toggleFolder(node.id)}>
                      {#if node.isOpen}<ChevronDown class="w-4 h-4 text-muted-foreground" />{:else}<ChevronRight class="w-4 h-4 text-muted-foreground" />{/if}
                      <Folder class="w-4 h-4 text-yellow-500" />
                      <span class="text-sm flex-1">{node.name}</span>
                      <div class="opacity-0 group-hover:opacity-100 flex gap-1 transition-opacity">
                        <Button variant="ghost" size="icon" class="h-6 w-6" onclick={(e) => { e.stopPropagation(); startCreateItem('file', node.id); }} disabled={isReviewer()}><FilePlus class="w-3 h-3" /></Button>
                        <Button variant="ghost" size="icon" class="h-6 w-6" onclick={(e) => { e.stopPropagation(); startCreateItem('folder', node.id); }} disabled={isReviewer()}><FolderPlus class="w-3 h-3" /></Button>
                        <Button variant="ghost" size="icon" class="h-6 w-6 text-destructive" onclick={(e) => { e.stopPropagation(); deleteFile(node.id); }} disabled={isReviewer()}><X class="w-3 h-3" /></Button>
                      </div>
                    </div>
                    {#if node.isOpen && node.children}
                      <div class="ml-4 space-y-1">
                        {#each node.children as child}
                          {#if child.type === 'folder'}
                            <div class="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted cursor-pointer" onclick={() => toggleFolder(child.id)}>
                              {#if child.isOpen}<ChevronDown class="w-4 h-4 text-muted-foreground" />{:else}<ChevronRight class="w-4 h-4 text-muted-foreground" />{/if}
                              <Folder class="w-4 h-4 text-yellow-500" />
                              <span class="text-sm flex-1">{child.name}</span>
                            </div>
                            {#if child.isOpen && child.children}
                              <div class="ml-6 space-y-1">
                                {#each child.children as grand}
                                  <div class="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted cursor-pointer group {activeFileId === grand.id ? 'bg-muted' : ''}" onclick={() => selectFile(grand.id)}>
                                    <FileText class="w-4 h-4 text-blue-500" /><span class="text-sm flex-1">{grand.name}</span>
                                    <Button variant="ghost" size="icon" class="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity" onclick={(e) => { e.stopPropagation(); deleteFile(grand.id); }} disabled={isReviewer()}><X class="w-3 h-3 text-destructive" /></Button>
                                  </div>
                                {/each}
                              </div>
                            {/if}
                          {:else}
                            <div class="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted cursor-pointer group {activeFileId === child.id ? 'bg-muted' : ''}" onclick={() => selectFile(child.id)}>
                              <FileText class="w-4 h-4 text-blue-500" /><span class="text-sm flex-1">{child.name}</span>
                              <Button variant="ghost" size="icon" class="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity" onclick={(e) => { e.stopPropagation(); deleteFile(child.id); }} disabled={isReviewer()}><X class="w-3 h-3 text-destructive" /></Button>
                            </div>
                          {/if}
                        {/each}
                      </div>
                    {/if}
                  {:else}
                    <div class="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-muted cursor-pointer group {activeFileId === node.id ? 'bg-muted' : ''}" onclick={() => selectFile(node.id)}>
                      <FileText class="w-4 h-4 text-blue-500" /><span class="text-sm flex-1">{node.name}</span>
                      <Button variant="ghost" size="icon" class="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity" onclick={(e) => { e.stopPropagation(); deleteFile(node.id); }} disabled={isReviewer()}><X class="w-3 h-3 text-destructive" /></Button>
                    </div>
                  {/if}
                </div>
              {/each}
            </div>
          </ScrollArea>
        </div>
      {/if}

      <div class="flex-1 min-h-0 flex flex-col border rounded-lg overflow-hidden shadow-sm">
        <div class="bg-muted px-4 py-2.5 text-sm font-mono border-b flex justify-between items-center">
          <span class="flex items-center gap-2"><FileText class="w-4 h-4" />{getActiveFile()?.name || 'untitled'}</span>
          <div class="flex items-center gap-2">
            <Badge variant="outline" class="text-xs font-mono">{getEditorLanguage()}</Badge>
            <Button variant="ghost" size="icon" class="h-6 w-6" title="Сохранить файл" onclick={saveWorkspace} disabled={isReviewer()}><Save class="w-3 h-3" /></Button>
          </div>
        </div>
        <Editor value={getEditorCode()} onValueChange={(v: string) => updateFileContent(v)} language={getEditorLanguage()} lines class="flex-1 font-mono text-sm" minHeight="300px" />
      </div>

      <div class="flex gap-3 items-center">
        <Button onclick={handleRun} class="gap-2" disabled={isReviewer()}><Play class="w-4 h-4" fill="currentColor" />Run Code</Button>
        <Button variant="secondary" onclick={() => updateFileContent('')} class="gap-2" disabled={isReviewer()}><Trash2 class="w-4 h-4" />Clear</Button>
        <Button variant="secondary" onclick={downloadWorkspace} class="gap-2"><Download class="w-4 h-4" />Export</Button>
        <Button onclick={submitWork} class="gap-2 ml-auto" disabled={isReviewer()}><Check class="w-4 h-4" />Сдать работу</Button>
        {#if submitStatus}<span class="text-xs text-muted-foreground">{submitStatus}</span>{/if}
      </div>
    </div>
  </div>
</div>
