import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Activity, ArrowDown, ArrowLeft, ArrowRight, ArrowUpRight, Bell, Check, CheckCheck, ChevronRight, CircleHelp, ClipboardCheck, Clock3, FlaskConical, HeartHandshake, History, LayoutDashboard, LoaderCircle, LockKeyhole, LogOut, MapPin, Phone, Play, Radio, RotateCcw, Save, ShieldCheck, Signal, Siren, UserRound, UsersRound, X } from 'lucide-react';
import { api, ApiError } from './api';
import type { Alert, CreateRequest, Dashboard, GeoLocation, Profile, Role, Status } from './types';

type View = 'overview' | 'profile' | 'history' | 'guide';
const statuses: Record<Status, { label: string; tone: string }> = {
  awaiting_response: { label: 'Ожидание ответа', tone: 'amber' },
  received: { label: 'Ожидает диспетчера', tone: 'red' },
  acknowledged: { label: 'Принято диспетчером', tone: 'green' },
  cancelled: { label: 'Отменено пользователем', tone: 'neutral' },
  closed: { label: 'Учебный случай закрыт', tone: 'neutral' },
};
const isActive = (a: Alert) => ['awaiting_response', 'received', 'acknowledged'].includes(a.status);
const date = (value: string) => new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
const time = (value: string) => new Intl.DateTimeFormat('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value));
const shortID = (id: string) => id.slice(0, 6).toUpperCase();
const message = (e: unknown) => e instanceof Error ? e.message : 'Не удалось выполнить действие.';

function Badge({ status }: { status: Status }) {
  return <span className={`badge ${statuses[status].tone}`}><span />{statuses[status].label}</span>;
}

function useDashboard(role: Role) {
  const [auth, setAuth] = useState<'checking' | 'needed' | 'ready'>('checking');
  const [data, setData] = useState<Dashboard | null>(null);
  const [connection, setConnection] = useState<'connecting' | 'live' | 'offline'>('connecting');
  const [error, setError] = useState('');
  const sequence = useRef(0);
  const mounted = useRef(true);
  const refresh = useCallback(async () => {
    const request = ++sequence.current;
    try {
      const next = await api<Dashboard>(`/${role}/dashboard`);
      if (mounted.current && request === sequence.current) { setData(next); setAuth('ready'); setError(''); }
      return next;
    } catch (e) {
      if (mounted.current && request === sequence.current) {
        if (e instanceof ApiError && e.status === 401) { setAuth('needed'); setData(null); }
        else { setError(message(e)); setConnection('offline'); }
      }
      return null;
    }
  }, [role]);
  useEffect(() => { mounted.current = true; void refresh(); return () => { mounted.current = false; sequence.current++; }; }, [refresh]);
  useEffect(() => {
    if (auth !== 'ready' || !data) return;
    const stream = new EventSource(`/api/${role}/events?cursor=${data.cursor}`);
    stream.addEventListener('ready', () => { setConnection('live'); void refresh(); });
    stream.addEventListener('change', () => { setConnection('live'); void refresh(); });
    stream.onerror = () => setConnection('offline');
    const fallback = window.setInterval(() => { void refresh(); }, 15000);
    return () => { stream.close(); window.clearInterval(fallback); };
    // The stream owns its cursor; reconnecting after every snapshot would race.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [auth, role, refresh]);
  const logout = async () => { await api('/auth/logout', 'POST', { role }); sequence.current++; setAuth('needed'); setData(null); };
  return { auth, data, connection, error, refresh, logout };
}

export default function App() {
  const role: Role = window.location.pathname.startsWith('/dispatcher') ? 'dispatcher' : 'user';
  return <Workspace key={role} role={role} />;
}

function Workspace({ role }: { role: Role }) {
  const { auth, data, connection, error, refresh, logout } = useDashboard(role);
  const [view, setView] = useState<View>('overview');
  const [toast, setToast] = useState('');
  const nav = [{ id: 'overview', name: role === 'user' ? 'Обзор' : 'Обращения', icon: LayoutDashboard }, ...(role === 'user' ? [{ id: 'profile', name: 'Моя карточка', icon: UserRound }] : []), { id: 'history', name: 'История сигналов', icon: History }, { id: 'guide', name: 'Как это работает', icon: CircleHelp }];
  useEffect(() => { if (!toast) return; const timer = window.setTimeout(() => setToast(''), 4500); return () => window.clearTimeout(timer); }, [toast]);
  const onLogout = async () => { try { await logout(); } catch (e) { setToast(message(e)); } };
  return <div className="app-shell">
    <aside className="sidebar">
      <a href={role === 'user' ? '/' : '/dispatcher'} className="brand" aria-label="Сигнал, главная"><span className="brand-mark"><Activity size={25} /></span><span>сигнал<span className="brand-dot">.</span></span></a>
      <div className="workspace-label">ПРОСТРАНСТВО ЗАБОТЫ</div>
      <nav aria-label="Основная навигация">{nav.map(n => <button key={n.id} className={`nav-item ${view === n.id ? 'selected' : ''}`} onClick={() => setView(n.id as View)}><n.icon size={19} /><span>{n.name}</span>{view === n.id && <ChevronRight size={15} />}</button>)}</nav>
      <div className="sidebar-note"><HeartHandshake size={25} strokeWidth={1.5} /><p>Рядом,<br /><strong>когда это важно.</strong></p><span>Меньше действий.<br />Больше ясности.</span></div>
      <div className="sidebar-bottom"><span className="avatar small">{role === 'user' ? 'АТ' : 'Д'}</span><div><strong>{role === 'user' ? 'Учебный пользователь' : 'Учебный диспетчер'}</strong><span>Локальная демонстрация</span></div>{auth === 'ready' && <button className="icon-btn" onClick={onLogout} aria-label="Выйти"><LogOut size={17} /></button>}</div>
    </aside>
    <div className="workspace">
      <header className="topbar"><div className="breadcrumb">{role === 'user' ? 'Личный кабинет' : 'Диспетчерская'}<ChevronRight size={14} /><strong>{nav.find(n => n.id === view)?.name}</strong></div><div className="topbar-actions">{auth === 'ready' && <span className={`connection ${connection}`} role="status"><span />{connection === 'live' ? 'Соединение активно' : connection === 'offline' ? 'Переподключение' : 'Подключаемся'}</span>}<a className="role-link" href={role === 'user' ? '/dispatcher' : '/'} target="_blank" rel="noreferrer">{role === 'user' ? 'Экран диспетчера' : 'Кабинет пользователя'}<ArrowUpRight size={15} /></a></div></header>
      <div className="training-banner"><FlaskConical size={16} /><strong>Учебный режим</strong><span>Тревоги получает только наш тестовый диспетчер. Связи с 103/112 нет.</span><span className="version">ПРОТОТИП 0.1</span></div>
      <main>
        {auth === 'checking' && !error && <div className="loading"><LoaderCircle className="spin" />Подключаемся к локальному серверу…</div>}
        {error && <div className="notice error" role="alert"><Signal size={20} /><div><strong>Не удаётся обновить данные</strong><p>{error}</p></div><button className="btn small secondary" onClick={() => void refresh()}><RotateCcw size={15} />Повторить</button></div>}
        {auth === 'needed' && <Login role={role} onLogin={refresh} />}
        {auth === 'ready' && data && <>
          {view === 'overview' && (role === 'user' ? <UserOverview data={data} refresh={refresh} notify={setToast} onProfile={() => setView('profile')} onHistory={() => setView('history')} /> : <Dispatcher data={data} refresh={refresh} notify={setToast} />)}
          {view === 'profile' && data.profile && <ProfileEditor initial={data.profile} onSaved={async () => { await refresh(); setToast('Учебная карточка сохранена'); }} />}
          {view === 'history' && <HistoryView alerts={data.alerts} />}
          {view === 'guide' && <Guide role={role} />}
        </>}
        <footer className="page-footer"><span><ShieldCheck size={14} />Только вымышленные данные для проверки сценария</span><span>Сигнал · Сделано с заботой</span></footer>
      </main>
    </div>
    {toast && <div className="toast" role="status"><CheckCheck size={18} />{toast}<button className="icon-btn" aria-label="Закрыть уведомление" onClick={() => setToast('')}><X size={16} /></button></div>}
  </div>;
}

function Login({ role, onLogin }: { role: Role; onLogin: () => Promise<unknown> }) {
  const defaultPassword = role === 'user' ? 'signal-user-demo' : 'signal-dispatcher-demo';
  const [password, setPassword] = useState(defaultPassword);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError('');
    try { await api('/auth/login', 'POST', { role, password }); await onLogin(); } catch (e) { setError(message(e)); } finally { setBusy(false); }
  }
  return <div className="login-layout"><div className="login-intro"><span className="eyebrow"><span className="tiny-line" />ПЕРВЫЙ ШАГ К СПОКОЙСТВИЮ</span><h1>Быть на связи.<br /><em>Даже когда сложно.</em></h1><p>Проверьте, как сигнал о помощи проходит от пользователя до учебного диспетчера.</p><div className="login-features"><span><ClipboardCheck size={19} />Данные под рукой</span><span><Radio size={19} />Понятный путь сигнала</span><span><UsersRound size={19} />Подтверждение от человека</span></div><div className="login-caption">Здесь мы проверяем идею.<br />Все обращения остаются внутри прототипа.</div></div><form className="login-card card" onSubmit={submit}><span className="large-icon">{role === 'user' ? <UserRound size={27} /> : <UsersRound size={27} />}</span><span className="eyebrow">ДЕМОНСТРАЦИОННЫЙ АККАУНТ</span><h2>{role === 'user' ? 'Кабинет пользователя' : 'Учебный диспетчер'}</h2><p>Пароль заполнен для локальной проверки. Используйте вымышленные сведения.</p><label className="field">Пароль<input autoComplete="current-password" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>{error && <p className="inline-error" role="alert">{error}</p>}<button className="btn primary full" disabled={busy}>{busy ? <LoaderCircle className="spin" size={18} /> : <ArrowRight size={18} />}Войти в демо</button><span className="login-credential">Учебный пароль: <code>{defaultPassword}</code></span><div className="login-security"><LockKeyhole size={15} />Отдельный сеанс для каждой роли</div></form></div>;
}

function Heading({ eyebrow, title, description, children }: { eyebrow: string; title: string; description: string; children?: ReactNode }) {
  return <div className="page-heading"><div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>{children}</div>;
}

function UserOverview({ data, refresh, notify, onProfile, onHistory }: { data: Dashboard; refresh: () => Promise<Dashboard | null>; notify: (s: string) => void; onProfile: () => void; onHistory: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [location, setLocation] = useState<GeoLocation | null>(null);
  const [geoState, setGeoState] = useState('');
  const [geoBusy, setGeoBusy] = useState(false);
  const [pending, setPending] = useState<CreateRequest | null>(() => { try { return JSON.parse(sessionStorage.getItem('signal-pending') || 'null'); } catch { return null; } });
  const p = data.profile!;
  const current = data.alerts.find(isActive);
  const offset = new Date(data.server_time).getTime() - Date.now();
  async function send(source: 'manual' | 'simulation', retry?: CreateRequest) {
    if (busy) return;
    setBusy(true); setError('');
    const req: CreateRequest = retry ?? (pending?.source === source ? pending : { source, client_request_id: crypto.randomUUID(), wait_seconds: source === 'simulation' ? 15 : 0, location: location && Date.now() - new Date(location.captured_at).getTime() < 9 * 60 * 1000 ? location : null });
    sessionStorage.setItem('signal-pending', JSON.stringify(req)); setPending(req);
    try { await api<Alert>('/user/alerts', 'POST', req); sessionStorage.removeItem('signal-pending'); setPending(null); await refresh(); notify(source === 'manual' ? 'Учебный SOS сохранён на сервере' : 'Учебная проверка запущена'); }
    catch (e) { if (e instanceof ApiError && e.status === 422) { sessionStorage.removeItem('signal-pending'); setPending(null); } setError(message(e)); } finally { setBusy(false); }
  }
  async function cancel() {
    if (!current) return; setBusy(true); setError('');
    try { await api(`/user/alerts/${current.id}/cancel`, 'POST', {}); await refresh(); notify('Учебная тревога отменена'); } catch (e) { setError(message(e)); await refresh(); } finally { setBusy(false); }
  }
  function locate() {
    if (!navigator.geolocation) { setGeoState('Браузер не поддерживает определение местоположения.'); return; }
    setGeoBusy(true); setGeoState('');
    navigator.geolocation.getCurrentPosition(pos => { setLocation({ latitude: pos.coords.latitude, longitude: pos.coords.longitude, accuracy: pos.coords.accuracy, captured_at: new Date(pos.timestamp).toISOString() }); setGeoBusy(false); setGeoState(''); }, () => { setGeoState('Местоположение не получено. Можно продолжить с учебным адресом из карточки.'); setGeoBusy(false); }, { enableHighAccuracy: false, timeout: 10000, maximumAge: 0 });
  }
  return <>
    <Heading eyebrow="ЛИЧНЫЙ КАБИНЕТ" title="Всё важное — рядом." description="Подготовьте карточку и проверьте путь сигнала о помощи."><span className="date-label">{new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' }).format(new Date())}</span></Heading>
    {pending && <div className="notice warning" role="status"><Clock3 size={21} /><div><strong>Осталась неподтверждённая отправка</strong><p>Повторная попытка использует тот же идентификатор и не создаёт дубликат.</p></div><button className="btn small secondary" disabled={busy} onClick={() => void send(pending.source, pending)}>Повторить</button></div>}
    {error && <div className="notice error" role="alert"><Signal size={19} /><p>{error}</p></div>}
    {current && <ActiveAlert alert={current} busy={busy} offset={offset} onCancel={cancel} onSOS={() => void send('manual')} />}
    <div className="overview-grid">
      <div className="overview-main">
        <section className="sos-card"><div className="sos-top"><span className="light-pill"><span />УЧЕБНЫЙ СИГНАЛ</span><Siren size={25} strokeWidth={1.5} /></div><h2>Когда нужна помощь,<br />важно быть услышанным.</h2><p>Один сигнал передаст вашу учебную карточку<br className="desktop-break" /> на экран диспетчера.</p><button className="sos-button" onClick={() => void send('manual')} disabled={busy || (current?.status === 'received' || current?.status === 'acknowledged')}><span className="sos-button-icon">{busy ? <LoaderCircle size={24} className="spin" /> : <Bell size={24} />}</span><span>{current?.status === 'received' || current?.status === 'acknowledged' ? 'Учебная тревога активна' : 'Отправить учебный SOS'}</span><ArrowRight size={20} /></button><div className="sos-bottom"><ShieldCheck size={15} />Только внутри прототипа · Без звонка в экстренные службы</div><Activity className="hero-watermark" aria-hidden="true" /></section>
        <section className="card simulation-card"><div className="section-icon"><FlaskConical size={22} /></div><div><span className="eyebrow">ПРОВЕРИТЬ АВТОМАТИЧЕСКИЙ СЦЕНАРИЙ</span><h3>А если человек не отвечает?</h3><p>Запустим учебное ожидание на 15 секунд. Если ответа нет, сервер создаст обращение для диспетчера.</p></div><button className="btn secondary" disabled={busy || !!current} onClick={() => void send('simulation')}><Play size={16} />Запустить</button><span className="simulation-footnote">Это имитация сигнала от устройства. Распознавание заболеваний не выполняется.</span></section>
        <section className="card recent-card"><div className="section-heading"><h3>Последние сигналы</h3><button className="text-button" onClick={onHistory}>Вся история<ArrowRight size={15} /></button></div>{data.alerts.length ? data.alerts.slice(0, 3).map(a => <div className="recent-row" key={a.id}><span className={`event-icon ${statuses[a.status].tone}`}><Activity size={18} /></span><div><strong>{a.source === 'manual' ? 'Учебный SOS' : 'Учебная проверка'}</strong><span>{date(a.created_at)} · № {shortID(a.id)}</span></div><Badge status={a.status} /></div>) : <div className="empty-inline"><span className="event-icon"><History size={20} /></span><div><strong>Здесь появится ваш первый сигнал</strong><p>Отправьте учебный SOS или проверьте автоматический сценарий.</p></div></div>}</section>
      </div>
      <div className="overview-aside">
        <section className="card profile-card"><div className="section-heading"><span className="eyebrow">МОЯ КАРТОЧКА</span><ClipboardCheck size={19} /></div><div className="profile-identity"><span className="avatar">{p.full_name.split(' ').slice(0, 2).map(v => v[0]).join('')}</span><div><h3>{p.full_name}</h3><span>Учебный профиль</span></div></div><div className="profile-divider" /><div className="info-line"><MapPin size={18} /><div><span>Сохранённый учебный адрес</span><p>{p.address}</p><small>{[p.apartment && `кв. ${p.apartment}`, p.entrance && `подъезд ${p.entrance}`, p.floor && `этаж ${p.floor}`].filter(Boolean).join(' · ')}</small></div></div><div className="info-line"><UsersRound size={18} /><div><span>Контакт из карточки</span><p>{p.contact_name || 'Не указан'}</p><small>{p.contact_phone || 'Телефон не указан'}</small></div></div><button className="btn secondary full" onClick={onProfile}>Посмотреть карточку<ArrowUpRight size={16} /></button><span className="card-caption">Снимок карточки сохраняется вместе с каждой тревогой.</span></section>
        <section className="card location-card"><div className="section-heading"><h3>Местоположение</h3><MapPin size={19} /></div>{location ? <><span className="location-status"><span />Координаты получены</span><p className="coordinates">{location.latitude.toFixed(5)}, {location.longitude.toFixed(5)}</p><p className="muted small-text">Точность ±{Math.round(location.accuracy)} м · {time(location.captured_at)}</p></> : <p className="muted">Текущие координаты пока не получены. Домашний адрес не считается текущим местом.</p>}<button className="text-button" onClick={locate} disabled={geoBusy}>{geoBusy ? <LoaderCircle size={15} className="spin" /> : <MapPin size={15} />}{geoBusy ? 'Определяем…' : location ? 'Обновить координаты' : 'Разрешить определение'}</button>{location && <button className="text-button muted" onClick={() => setLocation(null)}>Не передавать координаты</button>}{geoState && <p className="inline-error" role="status">{geoState}</p>}</section>
        <div className="quiet-note"><HeartHandshake size={24} /><p>Сначала — проверить.<br /><strong>Потом — доверять.</strong></p><span>Все действия можно пройти вместе с учебным диспетчером.</span></div>
      </div>
    </div>
  </>;
}

function ActiveAlert({ alert, busy, offset, onCancel, onSOS }: { alert: Alert; busy: boolean; offset: number; onCancel: () => void; onSOS: () => void }) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => { const t = window.setInterval(() => setNow(Date.now()), 250); return () => window.clearInterval(t); }, []);
  const remaining = alert.deadline_at ? Math.max(0, Math.ceil((new Date(alert.deadline_at).getTime() - now - offset) / 1000)) : 0;
  const waiting = alert.status === 'awaiting_response';
  return <section className={`active-alert ${statuses[alert.status].tone}`} aria-live="polite"><div className="active-icon">{waiting ? <Clock3 size={26} /> : alert.status === 'acknowledged' ? <CheckCheck size={26} /> : <Radio size={26} />}</div><div className="active-content"><div className="active-title"><h2>{waiting ? 'Вы в порядке?' : alert.status === 'received' ? 'Сигнал сохранён. Ждём диспетчера.' : 'Учебный диспетчер принял обращение.'}</h2><Badge status={alert.status} /></div><p>{waiting ? 'Учебный сценарий: подтвердите ответ или дождитесь отправки обращения.' : alert.status === 'received' ? 'Получение сервером подтверждено. Принятие человеком ещё не подтверждено.' : 'Это подтверждение только внутри прототипа. Бригада скорой помощи не вызвана.'}</p><div className="active-meta">№ {shortID(alert.id)}<span>Создано {time(alert.created_at)}</span></div></div><div className="active-actions">{waiting && <span className="countdown">{remaining > 0 ? `${remaining} с` : 'Отправляем…'}</span>}{(waiting || alert.status === 'received') && <button className="btn secondary small" disabled={busy || (waiting && remaining === 0)} onClick={onCancel}>{waiting ? 'Я в порядке' : 'Отменить учебный сигнал'}</button>}{waiting && <button className="text-button danger-text" disabled={busy} onClick={onSOS}>SOS без ожидания<ArrowRight size={14} /></button>}</div></section>;
}

const fieldGroups: { title: string; description: string; fields: { key: keyof Profile; label: string; type?: string; wide?: boolean; limit: number }[] }[] = [
  { title: 'Личные сведения', description: 'Как обратиться к человеку.', fields: [{ key: 'full_name', label: 'Имя и фамилия *', limit: 120 }, { key: 'birth_date', label: 'Дата рождения', type: 'date', limit: 10 }] },
  { title: 'Адрес и доступ', description: 'Это сохранённый адрес, а не подтверждённое текущее место.', fields: [{ key: 'address', label: 'Учебный адрес *', wide: true, limit: 500 }, { key: 'apartment', label: 'Квартира', limit: 30 }, { key: 'entrance', label: 'Подъезд', limit: 30 }, { key: 'floor', label: 'Этаж', limit: 30 }, { key: 'access_notes', label: 'Как найти вход, особенности доступа', type: 'textarea', wide: true, limit: 1000 }] },
  { title: 'Близкий человек', description: 'Контакт будет виден в карточке. Уведомления на этот номер не отправляются.', fields: [{ key: 'contact_name', label: 'Имя и отношение', limit: 120 }, { key: 'contact_phone', label: 'Учебный номер телефона', type: 'tel', limit: 60 }] },
  { title: 'Дополнительные сведения', description: 'Сведения из карточки не являются диагнозом или медицинским заключением.', fields: [{ key: 'conditions', label: 'Что важно знать о здоровье', type: 'textarea', wide: true, limit: 2000 }, { key: 'allergies', label: 'Аллергии', type: 'textarea', wide: true, limit: 2000 }] },
];

function ProfileEditor({ initial, onSaved }: { initial: Profile; onSaved: () => Promise<void> }) {
  const [draft, setDraft] = useState(initial);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { if (!dirty) setDraft(initial); }, [initial, dirty]);
  async function save(e: FormEvent) { e.preventDefault(); setBusy(true); setError(''); try { const p = await api<Profile>('/user/profile', 'PUT', draft); setDraft(p); setDirty(false); await onSaved(); } catch (e) { setError(message(e)); } finally { setBusy(false); } }
  return <><Heading eyebrow="ДАННЫЕ ПОД РУКОЙ" title="Моя учебная карточка" description="Заполните сведения заранее, чтобы они были доступны вместе с сигналом." /><div className="notice subtle"><FlaskConical size={19} /><p>Используйте только вымышленные данные. Карточка общая для демонстрационного аккаунта.</p></div><form className="profile-form" onSubmit={save}>{fieldGroups.map((group, i) => <section className="card form-section" key={group.title}><div className="form-section-heading"><span className="section-number">0{i + 1}</span><div><h3>{group.title}</h3><p>{group.description}</p></div></div><div className="form-grid">{group.fields.map(f => <label key={f.key} className={`field ${f.wide ? 'wide' : ''}`}>{f.label}{f.type === 'textarea' ? <textarea rows={3} maxLength={f.limit} value={draft[f.key] as string} onChange={e => { setDraft({ ...draft, [f.key]: e.target.value }); setDirty(true); }} /> : <input type={f.type ?? 'text'} maxLength={f.limit} required={f.key === 'full_name' || f.key === 'address'} value={draft[f.key] as string} onChange={e => { setDraft({ ...draft, [f.key]: e.target.value }); setDirty(true); }} />}</label>)}</div></section>)}{error && <p className="inline-error" role="alert">{error}</p>}<div className="form-bottom"><span>{dirty ? 'Есть несохранённые изменения' : `Сохранено ${date(initial.updated_at)}`}</span><button className="btn primary" disabled={busy || !dirty}>{busy ? <LoaderCircle size={17} className="spin" /> : <Save size={17} />}Сохранить карточку</button></div></form></>;
}

function Dispatcher({ data, refresh, notify }: { data: Dashboard; refresh: () => Promise<Dashboard | null>; notify: (s: string) => void }) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [filter, setFilter] = useState('active');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const filtered = data.alerts.filter(a => filter === 'all' || (filter === 'active' ? isActive(a) : !isActive(a)));
  const selected = filtered.find(a => a.id === selectedID) ?? filtered[0];
  async function action(kind: 'acknowledge' | 'close') {
    if (!selected) return; setBusy(true); setError('');
    try { await api(`/dispatcher/alerts/${selected.id}/${kind}`, 'POST', {}); await refresh(); notify(kind === 'acknowledge' ? 'Принятие учебной тревоги подтверждено' : 'Учебный случай закрыт'); } catch (e) { setError(message(e)); await refresh(); } finally { setBusy(false); }
  }
  return <><Heading eyebrow="УЧЕБНАЯ ДИСПЕТЧЕРСКАЯ" title="Каждый сигнал важен." description="Примите обращение и подтвердите пользователю, что его услышали."><button className="btn secondary small" onClick={() => void refresh()}><RotateCcw size={16} />Обновить</button></Heading><div className="dispatch-stats"><div><span className="stat-icon red"><Bell size={21} /></span><span><strong>{data.alerts.filter(a => a.status === 'received').length}</strong><small>Ожидают принятия</small></span></div><div><span className="stat-icon green"><UsersRound size={21} /></span><span><strong>{data.alerts.filter(a => a.status === 'acknowledged').length}</strong><small>В работе</small></span></div><div><span className="stat-icon"><CheckCheck size={21} /></span><span><strong>{data.alerts.filter(a => !isActive(a)).length}</strong><small>Завершено в выборке</small></span></div></div>{error && <div className="notice error" role="alert"><p>{error}</p></div>}<div className="dispatch-layout"><section className="card queue"><div className="queue-header"><h3>Обращения</h3><select aria-label="Фильтр обращений" value={filter} onChange={e => setFilter(e.target.value)}><option value="active">Активные</option><option value="all">Все</option><option value="done">Завершённые</option></select></div>{filtered.length ? filtered.map(a => <button key={a.id} className={`queue-item ${selected?.id === a.id ? 'selected' : ''}`} onClick={() => setSelectedID(a.id)}><div><span className="case-id">№ {shortID(a.id)}</span><time>{time(a.created_at)}</time></div><strong>{a.profile.full_name}</strong><span className="queue-address">{a.profile.address}</span><Badge status={a.status} /></button>) : <div className="queue-empty"><Radio size={32} strokeWidth={1.5} /><h3>Новых обращений пока нет</h3><p>Отправьте учебный SOS из кабинета пользователя.</p><a className="text-button" href="/" target="_blank" rel="noreferrer">Открыть кабинет<ArrowUpRight size={15} /></a></div>}</section>{selected ? <section className="card case-detail"><div className="case-heading"><div><span className="eyebrow">УЧЕБНОЕ ОБРАЩЕНИЕ № {shortID(selected.id)}</span><h2>{selected.profile.full_name}</h2></div><Badge status={selected.status} /></div><div className="case-summary"><ClipboardCheck size={21} /><div><strong>Сводка по карточке</strong><p>{selected.source === 'manual' ? 'Пользователь отправил учебный SOS.' : 'Запущена имитация сигнала от устройства.'} {selected.events.some(e => e.kind === 'response_timeout') ? 'В течение учебного окна ответ не получен.' : selected.status === 'awaiting_response' ? 'Сервер ожидает ответ пользователя; обращение ещё не передано на принятие.' : ''} Причина ухудшения состояния неизвестна. Автоматический диагноз не формируется.</p></div></div><div className="case-info-grid"><div className="case-info"><MapPin size={18} /><div><span>Сохранённый адрес из карточки</span><p>{selected.profile.address}</p><small>{[selected.profile.apartment && `кв. ${selected.profile.apartment}`, selected.profile.entrance && `подъезд ${selected.profile.entrance}`, selected.profile.floor && `этаж ${selected.profile.floor}`].filter(Boolean).join(' · ')}</small>{selected.profile.access_notes && <p>{selected.profile.access_notes}</p>}</div></div><div className="case-info"><Phone size={18} /><div><span>Контакт из карточки</span><p>{selected.profile.contact_name || 'Не указан'}</p><small>{selected.profile.contact_phone || 'Телефон не указан'} · уведомления не отправляются</small></div></div><div className="case-info"><HeartHandshake size={18} /><div><span>Сведения, указанные пользователем</span><p>{selected.profile.conditions || 'Не указаны'}</p><small>Аллергии: {selected.profile.allergies || 'не указаны'}</small></div></div><div className="case-info"><MapPin size={18} /><div><span>Координаты на момент сигнала</span>{selected.location ? <><p>{selected.location.latitude.toFixed(5)}, {selected.location.longitude.toFixed(5)}</p><small>±{Math.round(selected.location.accuracy)} м · {date(selected.location.captured_at)}</small></> : <p>Не получены. Текущее место не подтверждено.</p>}</div></div></div><div className="case-actions">{selected.status === 'received' && <button className="btn primary" disabled={busy} onClick={() => void action('acknowledge')}>{busy ? <LoaderCircle size={17} className="spin" /> : <Check size={18} />}Принять учебную тревогу</button>}{selected.status === 'acknowledged' && <button className="btn primary" disabled={busy} onClick={() => void action('close')}><CheckCheck size={18} />Завершить учебный случай</button>}{selected.status === 'awaiting_response' && <p><Clock3 size={17} />Ожидаем завершения учебной проверки</p>}{!isActive(selected) && <p><CheckCheck size={17} />Учебный случай завершён</p>}<span>Действия выполняются только внутри прототипа.</span></div><div className="timeline-heading"><h3>Путь сигнала</h3><span>Время на вашем устройстве</span></div><Timeline alert={selected} /></section> : <section className="card no-selection"><HeartHandshake size={50} strokeWidth={1.2} /><h2>На связи и готовы услышать.</h2><p>Здесь появятся карточка и история выбранного обращения.</p></section>}</div></>;
}

function Timeline({ alert }: { alert: Alert }) {
  const labels: Record<string, string> = { created: alert.source === 'manual' ? 'Учебный SOS сохранён на сервере' : 'Запущено учебное ожидание ответа', response_timeout: 'Ответ не получен — обращение готово к принятию', manual_override: 'Пользователь отправил SOS без ожидания', acknowledged: 'Учебный диспетчер принял обращение', cancelled: 'Пользователь отменил учебную тревогу', closed: 'Учебный случай закрыт' };
  return <ol className="timeline">{alert.events.map((e, i) => <li key={e.id}><span className={`timeline-dot ${i === alert.events.length - 1 ? 'last' : ''}`} /><div><strong>{labels[e.kind] || e.kind}</strong><span>{e.actor === 'user' ? 'Пользователь' : e.actor === 'dispatcher' ? 'Учебный диспетчер' : 'Система'}</span></div><time>{time(e.created_at)}</time></li>)}</ol>;
}

function HistoryView({ alerts }: { alerts: Alert[] }) {
  const [selected, setSelected] = useState<string | null>(null);
  const detail = alerts.find(a => a.id === selected);
  return <><Heading eyebrow="ИСТОРИЯ ОБРАЩЕНИЙ" title="Путь каждого сигнала." description="Последние 100 учебных обращений и подтверждённые действия по ним." /><section className="card history-card">{alerts.length ? <div className="history-list">{alerts.map(a => <button className={`history-row ${selected === a.id ? 'selected' : ''}`} key={a.id} onClick={() => setSelected(selected === a.id ? null : a.id)}><span className="event-icon"><Activity size={20} /></span><span><strong>{a.source === 'manual' ? 'Учебный SOS' : 'Учебная проверка'}</strong><small>№ {shortID(a.id)} · {date(a.created_at)}</small></span><Badge status={a.status} /><ChevronRight size={17} /></button>)}</div> : <div className="large-empty"><History size={37} strokeWidth={1.5} /><h2>Начните с первого учебного сигнала</h2><p>Его история сохранится здесь, даже если вы обновите страницу.</p></div>}</section>{detail && <section className="card history-detail"><div className="section-heading"><h3>Учебное обращение № {shortID(detail.id)}</h3><button className="icon-btn" aria-label="Закрыть историю обращения" onClick={() => setSelected(null)}><X size={18} /></button></div><Timeline alert={detail} /></section>}</>;
}

function Guide({ role }: { role: Role }) {
  const steps = [{ title: 'Подготовьте учебную карточку', text: 'В кабинете пользователя откройте «Моя карточка». Сохраните вымышленные сведения и учебный адрес.', icon: UserRound }, { title: 'Отправьте сигнал', text: 'Нажмите «Отправить учебный SOS». Для автоматической ветки запустите проверку и не отвечайте 15 секунд.', icon: Bell }, { title: 'Примите обращение', text: 'В другой вкладке откройте экран диспетчера, войдите в демо и нажмите «Принять учебную тревогу».', icon: UsersRound }, { title: 'Проверьте подтверждение', text: 'Вернитесь к пользователю: появится подтверждение. Обновите страницу — состояние и история сохранятся.', icon: CheckCheck }];
  return <><Heading eyebrow="ПОПРОБУЙТЕ ВМЕСТЕ" title="От сигнала — к ответу." description="Один сценарий, два экрана и понятное подтверждение каждого шага." /><div className="guide-grid">{steps.map((step, i) => <section className="card guide-step" key={step.title}><div><span className="section-number">0{i + 1}</span><step.icon size={23} /></div><h2>{step.title}</h2><p>{step.text}</p></section>)}</div><section className="card guide-limits"><h3>Что именно проверяет этот прототип</h3><p>Передачу и сохранение учебной тревоги, серверное ожидание ответа, принятие диспетчером и историю действий. Сводка составляется по шаблону из карточки; языковая модель пока не подключена.</p><p>Распознавание инсульта или инфаркта, реальный вызов 103/112, сообщения близким и постоянный фоновый мониторинг в эту версию не входят.</p><a className="btn primary" href={role === 'user' ? '/dispatcher' : '/'} target="_blank" rel="noreferrer">Открыть второй экран<ArrowUpRight size={17} /></a></section></>;
}
