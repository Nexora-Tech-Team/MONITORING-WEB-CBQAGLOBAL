import React, { useEffect, useMemo, useState } from 'react';

const STATUSES = [
  { value: 'todo', label: 'Todo' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'done', label: 'Done' },
];

const API = '/api';

function statusLabel(status) {
  return STATUSES.find((item) => item.value === status)?.label ?? status;
}

function normalize(text) {
  return String(text || '').toLowerCase();
}

function toDateKey(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString('en-CA');
}

function formatDateTime(value) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat('id-ID', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

async function fetchJSON(path, options) {
  const res = await fetch(`${API}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...(options?.headers || {}),
    },
    ...options,
  });
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json();
}

function App() {
  const [view, setView] = useState('dashboard');
  const [tasks, setTasks] = useState([]);
  const [stats, setStats] = useState(null);
  const [query, setQuery] = useState('');
  const [section, setSection] = useState('all');
  const [status, setStatus] = useState('all');
  const [dateFilter, setDateFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [savingId, setSavingId] = useState(null);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState('create');
  const [editingId, setEditingId] = useState(null);
  const [createSaving, setCreateSaving] = useState(false);
  const [createError, setCreateError] = useState('');
  const [taskForm, setTaskForm] = useState({
    pageSection: '',
    component: '',
    issue: '',
    status: 'todo',
  });

  async function loadData() {
    setLoading(true);
    setError('');
    try {
      const [taskData, statData] = await Promise.all([
        fetchJSON('/tasks?limit=1000'),
        fetchJSON('/stats'),
      ]);
      setTasks(taskData);
      setStats(statData);
    } catch (err) {
      setError(err.message || 'Failed to load data');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadData();
  }, []);

  const sections = useMemo(() => {
    return Array.from(new Set(tasks.map((task) => task.pageSection))).sort((a, b) =>
      a.localeCompare(b),
    );
  }, [tasks]);

  const filteredTasks = useMemo(() => {
    const q = normalize(query);
    return tasks.filter((task) => {
      const matchQuery =
        !q ||
        normalize(task.pageSection).includes(q) ||
        normalize(task.component).includes(q) ||
        normalize(task.issue).includes(q);
      const matchSection = section === 'all' || task.pageSection === section;
      const matchStatus = status === 'all' || task.status === status;
      const matchDate = !dateFilter || toDateKey(task.updatedAt) === dateFilter;
      return matchQuery && matchSection && matchStatus && matchDate;
    });
  }, [tasks, query, section, status, dateFilter]);

  const progress = useMemo(() => {
    if (!stats?.total) return 0;
    return Math.round((stats.done / stats.total) * 100);
  }, [stats]);

  async function updateTaskStatus(id, nextStatus) {
    setSavingId(id);
    try {
      const updated = await fetchJSON(`/tasks/${id}`, {
        method: 'PATCH',
        body: JSON.stringify({ status: nextStatus }),
      });
      setTasks((current) => current.map((task) => (task.id === id ? updated : task)));
      setStats((current) => {
        if (!current) return current;
        const clone = structuredClone(current);
        const prevTask = tasks.find((task) => task.id === id);
        if (prevTask) {
          if (prevTask.status === 'done') clone.done -= 1;
          if (prevTask.status === 'in_progress') clone.inProgress -= 1;
          if (prevTask.status === 'todo') clone.todo -= 1;
          if (prevTask.status === 'blocked') clone.blocked -= 1;
        }
        if (nextStatus === 'done') clone.done += 1;
        if (nextStatus === 'in_progress') clone.inProgress += 1;
        if (nextStatus === 'todo') clone.todo += 1;
        if (nextStatus === 'blocked') clone.blocked += 1;
        return clone;
      });
    } catch (err) {
      setError(err.message || 'Failed to save task');
    } finally {
      setSavingId(null);
    }
  }

  function openCreateForm() {
    setFormMode('create');
    setEditingId(null);
    setCreateError('');
    setTaskForm({
      pageSection: '',
      component: '',
      issue: '',
      status: 'todo',
    });
    setFormOpen(true);
  }

  function openEditForm(task) {
    setFormMode('edit');
    setEditingId(task.id);
    setCreateError('');
    setTaskForm({
      pageSection: task.pageSection,
      component: task.component,
      issue: task.issue,
      status: task.status,
    });
    setFormOpen(true);
  }

  function openTaskList(nextStatus = 'all', nextSection = 'all') {
    setQuery('');
    setSection(nextSection);
    setStatus(nextStatus);
    setDateFilter('');
    setFormOpen(false);
    setView('tasks');
  }

  async function saveTask(event) {
    event.preventDefault();
    setCreateSaving(true);
    setCreateError('');
    try {
      if (formMode === 'edit' && editingId !== null) {
        await fetchJSON(`/tasks/${editingId}`, {
          method: 'PATCH',
          body: JSON.stringify(taskForm),
        });
      } else {
        await fetchJSON('/tasks', {
          method: 'POST',
          body: JSON.stringify(taskForm),
        });
      }
      setFormOpen(false);
      setEditingId(null);
      setTaskForm({
        pageSection: '',
        component: '',
        issue: '',
        status: 'todo',
      });
      await loadData();
      setView('tasks');
    } catch (err) {
      setCreateError(err.message || 'Failed to save task');
    } finally {
      setCreateSaving(false);
    }
  }

  async function deleteTask(id) {
    const confirmed = window.confirm('Hapus task ini?');
    if (!confirmed) return;
    setSavingId(id);
    try {
      await fetchJSON(`/tasks/${id}`, {
        method: 'DELETE',
      });
      await loadData();
    } catch (err) {
      setError(err.message || 'Failed to delete task');
    } finally {
      setSavingId(null);
    }
  }

  return (
    <div className={sidebarCollapsed ? 'app-shell app-shell--collapsed' : 'app-shell'}>
      <aside className={sidebarCollapsed ? 'sidebar sidebar--collapsed' : 'sidebar'}>
        <div className="brand-block">
          <img src="/cbqa-logo.png" alt="CBQA Global" className="brand-logo" />
          <div>
            <p>CBQA Global</p>
            <strong>Monitoring Web</strong>
          </div>
        </div>

        <button
          type="button"
          className="sidebar-toggle"
          onClick={() => setSidebarCollapsed((current) => !current)}
        >
          {sidebarCollapsed ? 'Show Menu' : 'Hide Menu'}
        </button>

        <nav className="sidebar-nav">
          <button className={view === 'dashboard' ? 'nav-item active' : 'nav-item'} onClick={() => setView('dashboard')}>
            Dashboard
          </button>
          <button className={view === 'tasks' ? 'nav-item active' : 'nav-item'} onClick={() => setView('tasks')}>
            List Task
          </button>
        </nav>

        <div className="sidebar-note">
          <span>Status</span>
          <strong>{stats ? `${stats.done}/${stats.total} selesai` : 'Loading...'}</strong>
          <small>Tanpa login, status tersimpan di PostgreSQL.</small>
        </div>
      </aside>

      <main className="main-panel">
        <header className="hero-card">
          <div className="hero-card__overlay" />
          <div className="hero-card__content">
            <p className="eyebrow">CBQA Global Task Monitoring</p>
            <h1>Dashboard progres pekerjaan yang sederhana, cepat, dan terpusat.</h1>
            <p className="hero-copy">
              Monitoring Progress Pekerjaan{' '}
              <a className="hero-link" href="https://cbqaglobal.com" target="_blank" rel="noreferrer">
                https://cbqaglobal.com
              </a>
            </p>
            <div className="hero-actions">
              <button className={view === 'dashboard' ? 'button primary' : 'button'} onClick={() => setView('dashboard')}>
                Dashboard
              </button>
              <button className={view === 'tasks' ? 'button' : 'button secondary'} onClick={() => setView('tasks')}>
                Buka List Task
              </button>
            </div>
          </div>
          <div className="hero-card__art" aria-hidden="true">
            <img src="/background.png" alt="" />
          </div>
        </header>

        {error ? <div className="alert">{error}</div> : null}

        {view === 'dashboard' ? (
          <section className="dashboard-grid">
            <Metric
              title="Total Task"
              value={stats?.total ?? 0}
              hint="Semua item dari Excel"
              onClick={() => openTaskList('all')}
            />
            <Metric
              title="Done"
              value={stats?.done ?? 0}
              hint="Sudah selesai"
              onClick={() => openTaskList('done')}
            />
            <Metric
              title="In Progress"
              value={stats?.inProgress ?? 0}
              hint="Sedang dikerjakan"
              onClick={() => openTaskList('in_progress')}
            />
            <Metric
              title="Blocked"
              value={stats?.blocked ?? 0}
              hint="Perlu follow-up"
              onClick={() => openTaskList('blocked')}
            />

            <section className="panel panel--wide">
              <div className="panel-head">
                <div>
                  <p className="panel-kicker">Progress</p>
                  <h2>Ringkasan progres</h2>
                </div>
                <strong>{progress}%</strong>
              </div>
              <div className="progress-track">
                <div className="progress-bar" style={{ width: `${progress}%` }} />
              </div>
              <div className="summary-row">
                <span>{stats?.done ?? 0} selesai</span>
                <span>{stats?.todo ?? 0} belum dikerjakan</span>
                <span>{stats?.inProgress ?? 0} berjalan</span>
                <span>{stats?.blocked ?? 0} blocked</span>
              </div>
            </section>

            <section className="panel panel--wide">
              <div className="panel-head">
                <div>
                  <p className="panel-kicker">Section</p>
                  <h2>Top section workload</h2>
                </div>
              </div>
              <div className="section-list">
                {(stats?.bySection || []).slice(0, 10).map((item) => {
                  const percent = item.total > 0 ? Math.round((item.done / item.total) * 100) : 0;
                  return (
                    <button
                      key={item.name}
                      type="button"
                      className="section-row section-row--clickable"
                      onClick={() => openTaskList('all', item.name)}
                    >
                      <div className="section-row__topline">
                        <div>
                          <strong>{item.name}</strong>
                          <small>{item.done}/{item.total} done</small>
                        </div>
                        <span className="section-percent">{percent}%</span>
                      </div>
                      <div className="section-meter">
                        <span style={{ width: `${percent}%` }} />
                      </div>
                    </button>
                  );
                })}
              </div>
            </section>

            <section className="panel panel--wide">
              <div className="panel-head">
                <div>
                  <p className="panel-kicker">Recent</p>
                  <h2>Task terbaru diupdate</h2>
                </div>
              </div>
              <div className="recent-list">
                {(stats?.recentTasks || []).map((task) => (
                  <div key={task.id} className="recent-item">
                    <div>
                      <strong>{task.pageSection}</strong>
                      <p>{task.component}</p>
                    </div>
                    <span className={`badge badge--${task.status}`}>{statusLabel(task.status)}</span>
                  </div>
                ))}
              </div>
            </section>
          </section>
        ) : (
          <section className="panel panel--tasks">
            <div className="panel-head panel-head--stack">
              <div>
                <p className="panel-kicker">Task List</p>
                <h2>Ubah status pekerjaan langsung dari tabel</h2>
              </div>
              <div className="panel-actions">
                <button className="button secondary" onClick={loadData}>Refresh</button>
                <button className="button primary" onClick={openCreateForm}>
                  New Entry
                </button>
              </div>
            </div>

            {formOpen ? (
              <form className="create-task" onSubmit={saveTask}>
                <div className="panel-head panel-head--stack">
                  <div>
                    <p className="panel-kicker">{formMode === 'edit' ? 'Edit Entry' : 'Simple Entry'}</p>
                    <h2>{formMode === 'edit' ? 'Ubah data task' : 'Tambah data baru'}</h2>
                  </div>
                </div>
                <div className="create-task__grid">
                  <label>
                    Page / Section
                    <input
                      value={taskForm.pageSection}
                      onChange={(e) => setTaskForm((current) => ({ ...current, pageSection: e.target.value }))}
                      placeholder="Contoh: Homepage"
                      required
                    />
                  </label>
                  <label>
                    Component
                    <input
                      value={taskForm.component}
                      onChange={(e) => setTaskForm((current) => ({ ...current, component: e.target.value }))}
                      placeholder="Contoh: Button CTA"
                      required
                    />
                  </label>
                  <label className="create-task__full">
                    Issue
                    <textarea
                      rows="3"
                      value={taskForm.issue}
                      onChange={(e) => setTaskForm((current) => ({ ...current, issue: e.target.value }))}
                      placeholder="Tuliskan task baru"
                      required
                    />
                  </label>
                  <label>
                    Status
                    <select
                      value={taskForm.status}
                      onChange={(e) => setTaskForm((current) => ({ ...current, status: e.target.value }))}
                    >
                      {STATUSES.map((item) => (
                        <option key={item.value} value={item.value}>{item.label}</option>
                      ))}
                    </select>
                  </label>
                </div>
                {createError ? <div className="create-task__error">{createError}</div> : null}
                <div className="create-task__actions">
                  <button type="button" className="button secondary" onClick={() => setFormOpen(false)}>
                    Cancel
                  </button>
                  <button type="submit" className="button primary" disabled={createSaving}>
                    {createSaving ? 'Saving...' : formMode === 'edit' ? 'Save Changes' : 'Save Entry'}
                  </button>
                </div>
              </form>
            ) : null}

            <div className="filters">
              <label>
                Search
                <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Cari page, component, atau issue" />
              </label>
              <label>
                Section
                <select value={section} onChange={(e) => setSection(e.target.value)}>
                  <option value="all">All sections</option>
                  {sections.map((item) => (
                    <option key={item} value={item}>{item}</option>
                  ))}
                </select>
              </label>
              <label>
                Status
                <select value={status} onChange={(e) => setStatus(e.target.value)}>
                  <option value="all">All status</option>
                  {STATUSES.map((item) => (
                    <option key={item.value} value={item.value}>{item.label}</option>
                  ))}
                </select>
              </label>
              <label>
                Date
                <input
                  type="date"
                  value={dateFilter}
                  onChange={(e) => setDateFilter(e.target.value)}
                />
              </label>
            </div>

            <div className="table-wrap">
              <table className="task-table">
                <thead>
                  <tr>
                    <th>No</th>
                    <th className="col-section">Page / Section</th>
                    <th>Component</th>
                    <th className="col-issue">Issue</th>
                    <th className="col-status">Status</th>
                    <th className="col-date">Date</th>
                    <th className="col-actions">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {loading ? (
                    <tr><td colSpan={7} className="empty-state">Loading task...</td></tr>
                  ) : filteredTasks.length === 0 ? (
                    <tr><td colSpan={7} className="empty-state">Tidak ada task yang cocok dengan filter.</td></tr>
                  ) : (
                    filteredTasks.map((task) => (
                      <tr key={task.id}>
                        <td>{task.no}</td>
                        <td className="col-section">
                          <strong>{task.pageSection}</strong>
                        </td>
                        <td>{task.component}</td>
                        <td className="col-issue">{task.issue}</td>
                        <td className="col-status">
                          <div className="status-cell">
                            <span className={`badge badge--${task.status}`}>{statusLabel(task.status)}</span>
                            <select
                              value={task.status}
                              onChange={(e) => updateTaskStatus(task.id, e.target.value)}
                              disabled={savingId === task.id}
                            >
                              {STATUSES.map((item) => (
                                <option key={item.value} value={item.value}>{item.label}</option>
                              ))}
                            </select>
                          </div>
                        </td>
                        <td className="col-date">{formatDateTime(task.updatedAt)}</td>
                        <td className="col-actions">
                          <div className="row-actions">
                            <button type="button" className="button row-button" onClick={() => openEditForm(task)}>
                              Edit
                            </button>
                            <button
                              type="button"
                              className="button row-button danger"
                              onClick={() => deleteTask(task.id)}
                              disabled={savingId === task.id}
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </section>
        )}
      </main>
    </div>
  );
}

function Metric({ title, value, hint, onClick }) {
  return (
    <button type="button" className="metric-card" onClick={onClick}>
      <p>{title}</p>
      <strong>{value}</strong>
      <span>{hint}</span>
    </button>
  );
}

export default App;
