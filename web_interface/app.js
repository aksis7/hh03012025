// --- Конфиг ---
const baseURL = 'http://localhost:8080';

// DOM
const form = document.getElementById('task-form');
const urlsInput = document.getElementById('urls-input');

const activeSection = document.getElementById('active-section');
const tasksList = document.getElementById('tasks-list');
const noActive = document.getElementById('no-active');
const refreshActiveBtn = document.getElementById('refresh-active');

const pastSection = document.getElementById('past-section');
const pastList = document.getElementById('past-list');
const noPast = document.getElementById('no-past');
const togglePastBtn = document.getElementById('toggle-past');
const clearPastBtn = document.getElementById('clear-past');

// --- Хранилище в localStorage ---
const STORAGE_KEYS = {
  ACTIVE: 'dl_active_tasks',
  PAST: 'dl_past_tasks'
};

function loadActiveTasks() {
  try { return JSON.parse(localStorage.getItem(STORAGE_KEYS.ACTIVE) || '[]'); }
  catch { return []; }
}
function saveActiveTasks(arr) {
  localStorage.setItem(STORAGE_KEYS.ACTIVE, JSON.stringify(arr));
}
function loadPastTasks() {
  try { return JSON.parse(localStorage.getItem(STORAGE_KEYS.PAST) || '[]'); }
  catch { return []; }
}
function savePastTasks(arr) {
  localStorage.setItem(STORAGE_KEYS.PAST, JSON.stringify(arr));
}

// --- Рендер статуса ---
function translateStatus(status) {
  const normalized = (status || '').replace(/-/g, '-');
  switch (normalized) {
    case 'pending': return 'ожидает';
    case 'in-progress': return 'в процессе';
    case 'completed': return 'завершена';
    case 'completed_with_errors': return 'завершена с ошибками';
    case 'error': return 'ошибка';
    default: return status;
  }
}
function statusToBadgeClass(status) {
  const s = (status || '').replace(/-/g, '-');
  return `badge ${s}`;
}

// --- DOM helpers ---
function makeTaskElement(id, status, where = 'active') {
  const li = document.createElement('li');
  li.className = 'task-item';
  li.id = `${where}-task-${id}`;

  const header = document.createElement('div');
  header.className = 'task-header';

  const h3 = document.createElement('h3');
  h3.textContent = `Задача ${id}`;

  const meta = document.createElement('div');
  meta.className = 'task-meta';
  const badge = document.createElement('span');
  badge.className = statusToBadgeClass(status);
  badge.textContent = translateStatus(status);
  meta.appendChild(badge);

  header.appendChild(h3);
  header.appendChild(meta);

  const progress = document.createElement('p');
  progress.className = 'task-progress';
  progress.textContent = '';

  const filesUl = document.createElement('ul');
  filesUl.className = 'files-list';

  li.appendChild(header);
  li.appendChild(progress);
  li.appendChild(filesUl);

  return li;
}

function renderEmptyHints() {
  noActive.classList.toggle('hidden', tasksList.children.length > 0);
  noPast.classList.toggle('hidden', pastList.children.length > 0);
}

// --- Добавление/перемещение задач ---
function addActiveTask(id, status) {
  // Не дублируем
  if (document.getElementById(`active-task-${id}`)) return;
  const el = makeTaskElement(id, status, 'active');
  tasksList.prepend(el);
  renderEmptyHints();
}

function moveToPast(id, payload) {
  // Удаляем из активных
  const activeEl = document.getElementById(`active-task-${id}`);
  if (activeEl) activeEl.remove();

  // Создаём/обновляем карточку в «Прошедшие»
  let pastEl = document.getElementById(`past-task-${id}`);
  if (!pastEl) {
    pastEl = makeTaskElement(id, payload.status, 'past');
    pastList.prepend(pastEl);
  }
  // Обновим поля
  const badge = pastEl.querySelector('.badge');
  badge.className = statusToBadgeClass(payload.status);
  badge.textContent = translateStatus(payload.status);
  const progress = pastEl.querySelector('.task-progress');
  progress.textContent = `Завершено: ${payload.completed}/${payload.total}`;
  const filesUl = pastEl.querySelector('.files-list');
  filesUl.innerHTML = '';
  (payload.files || []).forEach((f) => {
    const li = document.createElement('li');
    li.className = 'file-item';
    const name = document.createElement('span');
    name.className = 'file-name';
    name.textContent = f.url;
    const st = document.createElement('span');
    const normalized = (f.status || '').replace(/-/g, '-');
    st.className = `file-status ${normalized}`;
    st.textContent = translateStatus(f.status);
    li.appendChild(name);
    li.appendChild(st);
    if (f.status === 'error' && f.error) {
      const err = document.createElement('span');
      err.className = 'file-error';
      err.textContent = ` (${f.error})`;
      li.appendChild(err);
    }
    filesUl.appendChild(li);
  });

  renderEmptyHints();

  // Переложим в localStorage
  const active = loadActiveTasks().filter(t => t.id !== id);
  saveActiveTasks(active);

  const past = loadPastTasks();
  // Сохраним краткую карточку истории
  const s = {
    id,
    status: payload.status,
    completed: payload.completed,
    total: payload.total,
    files: payload.files?.map(f => ({ url: f.url, status: f.status, error: f.error })) || [],
    updated_at: payload.updated_at || new Date().toISOString()
  };
  // заменим/вставим
  const idx = past.findIndex(x => x.id === id);
  if (idx >= 0) past[idx] = s; else past.unshift(s);
  savePastTasks(past);
}

// --- Опрос статусов активных задач ---
async function fetchTaskStatus(id) {
  const li = document.getElementById(`active-task-${id}`);
  // Если уже нет в активных — прервать
  if (!li) return;

  const badge = li.querySelector('.badge');
  const progress = li.querySelector('.task-progress');
  const filesUl = li.querySelector('.files-list');

  try {
    const res = await fetch(`${baseURL}/tasks/${id}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();

    // Обновляем активную карточку
    badge.className = statusToBadgeClass(data.status);
    badge.textContent = translateStatus(data.status);
    progress.textContent = `Завершено: ${data.completed}/${data.total}`;

    filesUl.innerHTML = '';
    (data.files || []).forEach((file) => {
      const liFile = document.createElement('li');
      liFile.className = 'file-item';
      const nameSpan = document.createElement('span');
      nameSpan.className = 'file-name';
      nameSpan.textContent = file.url;
      const fileStatusSpan = document.createElement('span');
      const normalizedStatus = (file.status || '').replace(/-/g, '-');
      fileStatusSpan.className = `file-status ${normalizedStatus}`;
      fileStatusSpan.textContent = translateStatus(file.status);
      liFile.appendChild(nameSpan);
      liFile.appendChild(fileStatusSpan);
      if (file.status === 'error' && file.error) {
        const errorSpan = document.createElement('span');
        errorSpan.className = 'file-error';
        errorSpan.textContent = ` (${file.error})`;
        liFile.appendChild(errorSpan);
      }
      filesUl.appendChild(liFile);
    });

    // Если задача завершена — переносим в «Прошедшие» и прекращаем опрос
    if (data.status === 'completed' || data.status === 'completed_with_errors' || data.status === 'error') {
      moveToPast(id, data);
      return;
    }

    // иначе продолжим опрос
    setTimeout(() => fetchTaskStatus(id), 3000);
  } catch (err) {
    console.error(err);
    progress.textContent = 'Ошибка получения статуса, повторяем...';
    setTimeout(() => fetchTaskStatus(id), 5000);
  }
}

// --- Создание задачи ---
form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const raw = urlsInput.value.trim();
  if (!raw) return;

  const urls = raw
    .split(/\r?\n/)
    .map((u) => u.trim())
    .filter((u) => u.length > 0);

  try {
    const res = await fetch(`${baseURL}/tasks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ urls }),
    });
    if (!res.ok) throw new Error(`Ошибка ${res.status}`);
    const data = await res.json();

    // В UI и localStorage
    addActiveTask(data.task_id, data.status);
    const active = loadActiveTasks();
    if (!active.find(t => t.id === data.task_id)) {
      active.unshift({ id: data.task_id, status: data.status, created_at: new Date().toISOString() });
      saveActiveTasks(active);
    }

    // Начинаем опрос
    fetchTaskStatus(data.task_id);
    urlsInput.value = '';
  } catch (err) {
    console.error(err);
    alert('Не удалось создать задачу. Проверьте корректность URL и доступность сервиса.');
  }
});

// --- Восстановление интерфейса при загрузке страницы ---
function restoreUI() {
  // Прошедшие
  const past = loadPastTasks();
  past.forEach(task => moveToPast(task.id, task)); // moveToPast сам обновит DOM и localStorage
  // Активные
  const active = loadActiveTasks();
  active.forEach(t => {
    addActiveTask(t.id, t.status || 'pending');
    // запускаем опрос
    fetchTaskStatus(t.id);
  });
  renderEmptyHints();
}

// --- Кнопки «Обновить активные», «Свернуть/Развернуть прошедшие», «Очистить прошедшие» ---
refreshActiveBtn.addEventListener('click', () => {
  const active = loadActiveTasks();
  if (active.length === 0) return;
  active.forEach(t => fetchTaskStatus(t.id));
});

togglePastBtn.addEventListener('click', () => {
  const expanded = togglePastBtn.getAttribute('aria-expanded') !== 'false';
  togglePastBtn.setAttribute('aria-expanded', expanded ? 'false' : 'true');
  pastList.style.display = expanded ? 'none' : '';
  noPast.style.display = expanded ? 'none' : '';
  togglePastBtn.textContent = expanded ? 'Развернуть' : 'Свернуть';
});

clearPastBtn.addEventListener('click', () => {
  if (!confirm('Очистить список прошедших задач?')) return;
  savePastTasks([]);
  pastList.innerHTML = '';
  renderEmptyHints();
});

// --- Инициализация ---
restoreUI();
