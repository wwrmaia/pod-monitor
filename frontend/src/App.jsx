import { useState, useEffect, useMemo, useRef } from 'react'
import axios from 'axios'
import './App.css'
import { translations, useT } from './i18n'
import GridLayout from 'react-grid-layout'
import 'react-grid-layout/css/styles.css'
import 'react-resizable/css/styles.css'
import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, AreaChart, Area } from 'recharts'
import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide } from 'd3'

function parseCPU(s) {
  if (!s || s === '0') return 0
  if (s.endsWith('n')) return parseFloat(s) / 1_000_000
  return s.endsWith('m') ? parseInt(s) : parseFloat(s) * 1000
}

function parseMem(s) {
  if (!s || s === '0') return 0
  if (s.endsWith('Gi')) return parseFloat(s) * 1024
  if (s.endsWith('Mi')) return parseFloat(s)
  if (s.endsWith('Ki')) return parseFloat(s) / 1024
  if (s.endsWith('G'))  return parseFloat(s) * 953.674
  return parseFloat(s)
}

function MetricCard({ label, value, alert }) {
  return (
    <div className='metric-card'>
      <div className='metric-label'>{label}</div>
      <div className={`metric-value ${alert ? 'alert' : ''}`}>{value}</div>
    </div>
  )
}

function TrendIcon({ trend }) {
  if (trend === 'up')   return <span className='trend up'>▲</span>
  if (trend === 'down') return <span className='trend down'>▼</span>
  return null
}

function calcPct(usageStr, limitStr, parseFn) {
  const usage = parseFn(usageStr)
  const limit = parseFn(limitStr)
  if (!limit)  return 'no-limit'
  if (!usage)  return 'no-data'
  return Math.round(usage / limit * 100)
}

function fmtPct(pct, noLimitLabel) {
  if (pct === 'no-limit') return noLimitLabel || 'sem limit'
  if (pct === 'no-data')  return '-'
  if (pct === 0)          return '< 1%'
  return `${pct}%`
}

function pctClass(pct) {
  if (pct === 'no-limit') return 'val-muted'
  if (typeof pct !== 'number') return ''
  if (pct >= 90) return 'val-alert'
  if (pct >= 85) return 'val-caution'
  return ''
}

function dotStatus(cpuPct, memPct) {
  const pcts = [cpuPct, memPct].filter(p => typeof p === 'number')
  if (pcts.some(p => p >= 90)) return 'warn'
  if (pcts.some(p => p >= 85)) return 'caution'
  return 'ok'
}

function fmtTime(iso) {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' })
}

// ── Login ─────────────────────────────────────────────────────────────────────
function LoginPage({ onLogin, lang }) {
  const t = useT(lang || 'pt')
  const [username,     setUsername]     = useState('')
  const [password,     setPassword]     = useState('')
  const [step,         setStep]         = useState('login') // 'login' | 'mfa' | 'mfa_setup'
  const [mfaToken,     setMfaToken]     = useState('')
  const [mfaCode,      setMfaCode]      = useState('')
  const [setupToken,   setSetupToken]   = useState('')
  const [setupData,    setSetupData]    = useState(null)  // { qr_code, secret, username }
  const [setupCode,    setSetupCode]    = useState('')
  const [error,        setError]        = useState('')
  const [loading,      setLoading]      = useState(false)

  const logo = (
    <div className='login-logo'>
      <svg viewBox='0 0 24 24' fill='none'>
        <path d='M12 2.5 L21.5 7.5 L12 12.5 L2.5 7.5 Z'
          fill='rgba(255,255,255,0.28)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
        <path d='M2.5 7.5 L2.5 16 L12 21 L12 12.5 Z'
          fill='rgba(255,255,255,0.12)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
        <path d='M21.5 7.5 L21.5 16 L12 21 L12 12.5 Z'
          fill='rgba(0,0,0,0.18)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
        <circle cx='8.8'  cy='7.2'  r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
        <circle cx='15.2' cy='7.2'  r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
        <circle cx='12'   cy='10.2' r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
      </svg>
    </div>
  )

  async function handleSubmit(e) {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      const { data } = await axios.post('/api/auth/login', { username, password })
      if (data.mfa_required) {
        setMfaToken(data.mfa_token); setStep('mfa')
      } else if (data.mfa_setup_required) {
        // Busca QR code imediatamente
        const { data: qr } = await axios.get(`/api/auth/mfa/setup?token=${data.setup_token}`)
        setSetupToken(data.setup_token); setSetupData(qr); setStep('mfa_setup')
      } else {
        onLogin(data)
      }
    } catch (err) {
      setError(err.response?.data?.error || err.response?.data || t('loginError'))
    } finally {
      setLoading(false)
    }
  }

  async function handleMFASubmit(e) {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      const { data } = await axios.post('/api/auth/mfa/validate', { mfa_token: mfaToken, code: mfaCode })
      onLogin(data)
    } catch (err) {
      setError(err.response?.data?.error || t('mfaInvalid'))
      setMfaCode('')
    } finally {
      setLoading(false)
    }
  }

  async function handleSetupSubmit(e) {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      const { data } = await axios.post('/api/auth/mfa/setup/confirm', { setup_token: setupToken, code: setupCode })
      onLogin(data)
    } catch (err) {
      setError(err.response?.data?.error || t('codeInvalid'))
      setSetupCode('')
    } finally {
      setLoading(false)
    }
  }

  // ── Step: verificar MFA ──
  if (step === 'mfa') {
    return (
      <div className='login-overlay'>
        <div className='login-box'>
          {logo}
          <h1 className='login-title'>{t('mfaVerification')}</h1>
          <p className='login-mfa-hint'>{t('mfaHint')}</p>
          <form className='login-form' onSubmit={handleMFASubmit}>
            <div className='login-field'>
              <label>{t('mfaCode')}</label>
              <input type='text' autoFocus autoComplete='one-time-code' inputMode='numeric'
                maxLength={6} value={mfaCode} placeholder='000000'
                onChange={e => setMfaCode(e.target.value.replace(/\D/g, ''))} />
            </div>
            {error && <div className='login-error'>{error}</div>}
            <button className='login-btn' type='submit' disabled={loading || mfaCode.length !== 6}>
              {loading ? t('verifying') : t('verify')}
            </button>
            <button type='button' className='login-back-btn'
              onClick={() => { setStep('login'); setMfaCode(''); setError('') }}>
              {t('back')}
            </button>
          </form>
        </div>
      </div>
    )
  }

  // ── Step: cadastrar MFA (primeiro acesso) ──
  if (step === 'mfa_setup' && setupData) {
    return (
      <div className='login-overlay'>
        <div className='login-box login-box-setup'>
          {logo}
          <h1 className='login-title'>{t('configureMfa')}</h1>
          <p className='login-mfa-hint'>
            {t('mfaSetupHint').split('\n').map((line, i) => <span key={i}>{line}{i === 0 && <br/>}</span>)}
          </p>
          <img className='mfa-qr-img' src={setupData.qr_code} alt='QR Code MFA' style={{margin:'8px 0'}} />
          <div className='mfa-secret-label'>{t('mfaManualKey')}</div>
          <code className='mfa-secret'>{setupData.secret}</code>
          <form className='login-form' style={{marginTop:'12px'}} onSubmit={handleSetupSubmit}>
            <div className='login-field'>
              <label>{t('confirmWithCode')}</label>
              <input type='text' autoFocus autoComplete='one-time-code' inputMode='numeric'
                maxLength={6} value={setupCode} placeholder='000000'
                onChange={e => setSetupCode(e.target.value.replace(/\D/g, ''))} />
            </div>
            {error && <div className='login-error'>{error}</div>}
            <button className='login-btn' type='submit' disabled={loading || setupCode.length !== 6}>
              {loading ? t('confirming') : t('confirmAndEnter')}
            </button>
          </form>
        </div>
      </div>
    )
  }

  // ── Step: login normal ──
  return (
    <div className='login-overlay'>
      <div className='login-box'>
        {logo}
        <h1 className='login-title'>Pod Resource Monitor</h1>
        <form className='login-form' onSubmit={handleSubmit}>
          <div className='login-field'>
            <label>{t('username')}</label>
            <input type='text' autoFocus value={username} onChange={e => setUsername(e.target.value)} />
          </div>
          <div className='login-field'>
            <label>{t('password')}</label>
            <input type='password' value={password} onChange={e => setPassword(e.target.value)} />
          </div>
          {error && <div className='login-error'>{error}</div>}
          <button className='login-btn' type='submit' disabled={loading || !username || !password}>
            {loading ? t('loggingIn') : t('login')}
          </button>
        </form>
      </div>
    </div>
  )
}

// ── Seletor de acesso Dev (clusters + namespaces com checkboxes) ───────────────
function DevAccessPicker({ selClusters, selNamespaces, onChange, lang }) {
  const t = useT(lang || 'pt')
  const [availClusters, setAvailClusters] = useState([])
  const [allNamespaces, setAllNamespaces] = useState([])
  const [nsLoading,     setNsLoading]     = useState(false)

  useEffect(() => {
    axios.get('/api/clusters').then(({ data }) => {
      const list = data || []
      setAvailClusters(list)
      if (list.length > 0) loadNamespaces(list)
    }).catch(() => {})
  }, [])

  async function loadNamespaces(clusterList) {
    setNsLoading(true)
    try {
      const results = await Promise.all(
        clusterList.map(c => axios.get(`/api/namespaces?cluster=${c}`).then(r => r.data || []).catch(() => []))
      )
      setAllNamespaces([...new Set(results.flat())].sort())
    } finally { setNsLoading(false) }
  }

  function toggleCluster(c) {
    const next = selClusters.includes(c) ? selClusters.filter(x => x !== c) : [...selClusters, c]
    onChange({ clusters: next, namespaces: selNamespaces })
  }

  function toggleNamespace(n) {
    const next = selNamespaces.includes(n) ? selNamespaces.filter(x => x !== n) : [...selNamespaces, n]
    onChange({ clusters: selClusters, namespaces: next })
  }

  return (
    <div className='dev-access-picker'>
      <div className='dap-section'>
        <div className='dap-title'>{t('allowedClusters')}</div>
        <div className='dap-checks'>
          {availClusters.length === 0 && <span className='dap-empty'>{t('noClusterAvailable')}</span>}
          {availClusters.map(c => (
            <label key={c} className='dap-item'>
              <input type='checkbox' checked={selClusters.includes(c)} onChange={() => toggleCluster(c)} />
              {c}
            </label>
          ))}
        </div>
      </div>
      <div className='dap-section'>
        <div className='dap-title'>{t('allowedNamespaces')}</div>
        {nsLoading
          ? <span className='dap-empty'>{t('loading')}</span>
          : <div className='dap-checks'>
              {allNamespaces.length === 0 && <span className='dap-empty'>{t('noNamespaceAvailable')}</span>}
              {allNamespaces.map(n => (
                <label key={n} className='dap-item'>
                  <input type='checkbox' checked={selNamespaces.includes(n)} onChange={() => toggleNamespace(n)} />
                  {n}
                </label>
              ))}
            </div>
        }
      </div>
    </div>
  )
}

// ── Modal de edição de acesso Dev ─────────────────────────────────────────────
// onSaved(username, clusters, namespaces) — a chamada de API fica no pai
function DevAccessModal({ user, onClose, onSaved, lang }) {
  const t = useT(lang || 'pt')
  const [selClusters,   setSelClusters]   = useState(user.allowed_clusters   || [])
  const [selNamespaces, setSelNamespaces] = useState(user.allowed_namespaces || [])
  const [saving,        setSaving]        = useState(false)
  const [msg,           setMsg]           = useState({ type: '', text: '' })

  function handleChange({ clusters, namespaces }) {
    setSelClusters(clusters); setSelNamespaces(namespaces)
  }

  async function save() {
    setSaving(true); setMsg({ type: '', text: '' })
    try {
      await onSaved(user.username, selClusters, selNamespaces)
    } catch (err) {
      setMsg({ type: 'error', text: err.response?.data?.error || t('errorSavingAccess') })
    } finally { setSaving(false) }
  }

  return (
    <div className='modal-overlay' onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className='modal-box'>
        <div className='modal-header'>
          <span>{t('devAccessTitle')} — <strong>{user.username}</strong></span>
          <button className='modal-close' onClick={onClose}>✕</button>
        </div>
        <div className='modal-body'>
          <DevAccessPicker selClusters={selClusters} selNamespaces={selNamespaces} onChange={handleChange} lang={lang} />
          {msg.text && <div className={`um-msg ${msg.type}`} style={{ marginTop: '0.75rem' }}>{msg.text}</div>}
        </div>
        <div className='modal-footer'>
          <button onClick={onClose}>{t('cancel')}</button>
          <button className='modal-save' onClick={save} disabled={saving}>
            {saving ? t('saving') : t('save')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Página de gerenciamento de usuários ──────────────────────────────────────
function UserManagementPage({ onBack, lang }) {
  const t = useT(lang || 'pt')
  const [users,        setUsers]        = useState([])
  const [groups,       setGroups]       = useState([])
  const [createForm,   setCreateForm]   = useState({ username: '', password: '', group_name: '', role: 'administration' })
  const [createMsg,    setCreateMsg]    = useState({ type: '', text: '' })
  const [createLoad,   setCreateLoad]   = useState(false)
  const [pwdTarget,    setPwdTarget]    = useState('')
  const [pwdValue,     setPwdValue]     = useState('')
  const [pwdMsg,       setPwdMsg]       = useState({ type: '', text: '' })
  const [pwdLoad,      setPwdLoad]      = useState(false)
  const [mfaQRModal,   setMfaQRModal]   = useState(null) // { group, qr_codes: [{username,qr_code,secret}] }
  const [mfaLoading,   setMfaLoading]   = useState('')
  const [groupForm,    setGroupForm]    = useState({ name: '', role: 'reader' })
  const [groupMsg,     setGroupMsg]     = useState({ type: '', text: '' })
  const [groupLoad,    setGroupLoad]    = useState(false)
  const [editAccess,   setEditAccess]   = useState(null) // group object em edição
  const [addMember,    setAddMember]    = useState({ group: '', username: '' })
  const [editRole,     setEditRole]     = useState(null) // username being role-edited

  useEffect(() => { loadAll() }, [])

  async function loadAll() {
    try {
      const [ur, gr] = await Promise.all([
        axios.get('/api/auth/users'),
        axios.get('/api/auth/groups'),
      ])
      setUsers(ur.data || [])
      setGroups(gr.data || [])
    } catch {}
  }

  // ── Usuários ──
  async function createUser(e) {
    e.preventDefault()
    setCreateLoad(true); setCreateMsg({ type: '', text: '' })
    try {
      const payload = { username: createForm.username, password: createForm.password }
      if (createForm.group_name) payload.group_name = createForm.group_name
      else payload.role = createForm.role
      await axios.post('/api/auth/users/create', payload)
      setCreateMsg({ type: 'ok', text: t('userCreated') })
      setCreateForm({ username: '', password: '', group_name: '', role: 'administration' })
      loadAll()
    } catch (err) {
      setCreateMsg({ type: 'error', text: err.response?.data?.error || t('errorCreatingUser') })
    } finally { setCreateLoad(false) }
  }

  async function deleteUser(username) {
    if (!confirm(t('removeConfirmUser')(username))) return
    try {
      await axios.delete(`/api/auth/users/delete?username=${username}`)
      loadAll()
    } catch (err) {
      setCreateMsg({ type: 'error', text: err.response?.data?.error || t('errorDeletingUser') })
    }
  }

  async function changePassword(e) {
    e.preventDefault()
    setPwdLoad(true); setPwdMsg({ type: '', text: '' })
    try {
      await axios.post('/api/auth/users/password', { username: pwdTarget, password: pwdValue })
      setPwdMsg({ type: 'ok', text: t('passwordChanged')(pwdTarget) })
      setPwdTarget(''); setPwdValue('')
    } catch (err) {
      setPwdMsg({ type: 'error', text: err.response?.data?.error || t('errorSavingPassword') })
    } finally { setPwdLoad(false) }
  }

  // MFA individual (apenas para admin sem grupo)
  async function toggleAdminMFA(u) {
    const enabling = !u.totp_enabled
    setMfaLoading(u.username)
    try {
      await axios.post('/api/auth/mfa/toggle', { username: u.username, enabled: enabling })
      setUsers(prev => prev.map(x => x.username === u.username ? { ...x, totp_enabled: enabling, totp_configured: enabling ? x.totp_configured : false } : x))
      if (enabling) setCreateMsg({ type: 'ok', text: t('mfaEnabled') })
    } catch (err) {
      setCreateMsg({ type: 'error', text: err.response?.data?.error || t('errorChangingMfa') })
    } finally { setMfaLoading('') }
  }

  // ── Grupos ──
  async function createGroup(e) {
    e.preventDefault()
    setGroupLoad(true); setGroupMsg({ type: '', text: '' })
    try {
      await axios.post('/api/auth/groups/create', { name: groupForm.name, role: groupForm.role })
      setGroupMsg({ type: 'ok', text: t('groupCreated')(groupForm.name) })
      setGroupForm({ name: '', role: 'reader' })
      loadAll()
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorCreatingGroup') })
    } finally { setGroupLoad(false) }
  }

  async function deleteGroup(name) {
    if (!confirm(t('removeConfirmGroup')(name))) return
    try {
      await axios.delete(`/api/auth/groups/delete?name=${encodeURIComponent(name)}`)
      loadAll()
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorDeletingGroup') })
    }
  }

  async function toggleGroupMFA(g) {
    const enabling = !g.totp_enabled
    setMfaLoading(`group:${g.name}`)
    try {
      await axios.post('/api/auth/groups/mfa', { name: g.name, enabled: enabling })
      setGroups(prev => prev.map(x => x.name === g.name ? { ...x, totp_enabled: enabling } : x))
      if (enabling) setGroupMsg({ type: 'ok', text: t('groupMfaEnabled')(g.name) })
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorChangingGroupMfa') })
    } finally { setMfaLoading('') }
  }

  async function resetUserMFA(username) {
    if (!confirm(t('resetMfaConfirm')(username))) return
    try {
      await axios.post('/api/auth/mfa/reset-user', { username })
      setUsers(prev => prev.map(u => u.username === username ? { ...u, totp_configured: false } : u))
      setGroupMsg({ type: 'ok', text: t('mfaReset')(username) })
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorResettingMfa') })
    }
  }

  async function changeUserRole(username, role) {
    try {
      await axios.post('/api/auth/users/role', { username, role })
      setUsers(prev => prev.map(u => u.username === username ? { ...u, role } : u))
      setEditRole(null)
    } catch (err) {
      setCreateMsg({ type: 'error', text: err.response?.data?.error || t('errorChangingProfile') })
    }
  }

  async function addMemberToGroup(e) {
    e.preventDefault()
    try {
      await axios.post('/api/auth/groups/members', { group: addMember.group, username: addMember.username, action: 'add' })
      setAddMember({ group: '', username: '' })
      loadAll()
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorAddingMember') })
    }
  }

  async function removeMemberFromGroup(group, username) {
    try {
      await axios.post('/api/auth/groups/members', { group, username, action: 'remove' })
      loadAll()
    } catch (err) {
      setGroupMsg({ type: 'error', text: err.response?.data?.error || t('errorRemovingMember') })
    }
  }

  async function handleGroupAccessSaved(_, clusters, namespaces) {
    await axios.post('/api/auth/groups/access', { name: editAccess.name, allowed_clusters: clusters, allowed_namespaces: namespaces })
    loadAll()
    setEditAccess(null)
  }

  const roleLabel = r => r === 'administration' ? 'Administration' : r === 'dev' ? 'Dev' : 'Reader'
  const roleCls   = r => r === 'administration' ? 'admin' : r === 'dev' ? 'dev' : 'reader'

  // Adaptador para DevAccessModal que aceita um grupo
  const groupAsUser = g => g ? {
    username: g.name, role: g.role,
    allowed_clusters: g.allowed_clusters || [],
    allowed_namespaces: g.allowed_namespaces || [],
  } : null

  return (
    <div className='um-page'>
      {/* Modal DevAccess para grupos */}
      {editAccess && (
        <DevAccessModal
          user={groupAsUser(editAccess)}
          onClose={() => setEditAccess(null)}
          onSaved={handleGroupAccessSaved}
        />
      )}

      {/* Modal QR Codes MFA */}
      {mfaQRModal && (
        <div className='mfa-modal-overlay' onClick={() => setMfaQRModal(null)}>
          <div className='mfa-modal mfa-modal-wide' onClick={e => e.stopPropagation()}>
            <div className='mfa-modal-title'>
              {t('mfaQrTitle')(mfaQRModal.group)}
            </div>
            <p className='mfa-modal-desc'>{t('mfaQrDesc')}</p>
            <div className='mfa-qr-list'>
              {mfaQRModal.qr_codes.map(q => (
                <div key={q.username} className='mfa-qr-item'>
                  <div className='mfa-qr-username'>{q.username}</div>
                  <img className='mfa-qr-img' src={q.qr_code} alt={`QR ${q.username}`} />
                  <code className='mfa-secret'>{q.secret}</code>
                </div>
              ))}
            </div>
            <button className='admin-btn' style={{marginTop:'1rem'}} onClick={() => setMfaQRModal(null)}>{t('close')}</button>
          </div>
        </div>
      )}

      <div className='um-header'>
        <button className='um-back' onClick={onBack}>
          <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
            <polyline points='15 18 9 12 15 6'/>
          </svg>
          {t('backBtn')}
        </button>
        <h2>{t('manageUsers')}</h2>
      </div>

      <div className='um-body'>

        {/* ── Grupos ── */}
        <div className='um-card'>
          <div className='um-card-title'>{t('groups')}</div>
          {groupMsg.text && <div className={`um-msg ${groupMsg.type}`}>{groupMsg.text}</div>}
          {groups.length === 0 && <div className='empty' style={{padding:'12px 18px'}}>{t('noGroupsRegistered')}</div>}
          {groups.map(g => (
            <div key={g.name} className='group-card'>
              <div className='group-card-header'>
                <div className='group-card-title'>
                  <span className='group-name'>{g.name}</span>
                </div>
                <div className='group-card-actions'>
                  <button className='um-btn-access' onClick={() => setEditAccess(g)}>{t('editAccess')}</button>
                  <div className='group-mfa-wrap'>
                    <span className='group-mfa-label'>{t('mfaLabel')}</span>
                    <label className={`mfa-switch ${mfaLoading === `group:${g.name}` ? 'mfa-switch-loading' : ''}`}>
                      <input
                        type='checkbox'
                        checked={!!g.totp_enabled}
                        disabled={mfaLoading === `group:${g.name}`}
                        onChange={() => toggleGroupMFA(g)}
                      />
                      <span className='mfa-slider' />
                    </label>
                  </div>
                  <button className='um-btn-del' onClick={() => deleteGroup(g.name)}>{t('deleteGroup')}</button>
                </div>
              </div>
              <div className='group-members'>
                {g.members?.length > 0 ? (
                  g.members.map(m => (
                    <span key={m} className='group-member-tag'>
                      {m}
                      <button className='group-member-remove' onClick={() => removeMemberFromGroup(g.name, m)} title={t('removeFromGroup')}>×</button>
                    </span>
                  ))
                ) : (
                  <span className='group-no-members'>{t('noMembers')}</span>
                )}
              </div>
              {(g.allowed_clusters?.length || g.allowed_namespaces?.length) ? (
                <div className='group-access-info'>
                  Clusters: {g.allowed_clusters?.join(', ') || '—'} &nbsp;·&nbsp;
                  Namespaces: {g.allowed_namespaces?.join(', ') || '—'}
                </div>
              ) : null}
            </div>
          ))}

          {/* Adicionar membro a grupo */}
          <form className='um-form' style={{borderTop:'1px solid var(--border)',marginTop:'10px'}} onSubmit={addMemberToGroup}>
            <div className='um-card-title' style={{padding:'10px 18px 4px'}}>{t('addMemberToGroup')}</div>
            <div className='um-form-row'>
              <div className='um-form-field'>
                <label>{t('group')}</label>
                <select value={addMember.group} onChange={e => setAddMember(f => ({...f, group: e.target.value}))} required>
                  <option value=''>{t('selectGroup')}</option>
                  {groups.map(g => <option key={g.name} value={g.name}>{g.name}</option>)}
                </select>
              </div>
              <div className='um-form-field'>
                <label>{t('userCol')}</label>
                <select value={addMember.username} onChange={e => setAddMember(f => ({...f, username: e.target.value}))} required>
                  <option value=''>{t('selectUser')}</option>
                  {users.filter(u => u.role !== 'administration').map(u => (
                    <option key={u.username} value={u.username}>{u.username}{u.group_name ? ` (${u.group_name})` : ''}</option>
                  ))}
                </select>
              </div>
              <div className='um-form-field um-form-action'>
                <label>&nbsp;</label>
                <button type='submit' disabled={!addMember.group || !addMember.username}>{t('add')}</button>
              </div>
            </div>
          </form>

          {/* Criar grupo */}
          <form className='um-form' style={{borderTop:'1px solid var(--border)'}} onSubmit={createGroup}>
            <div className='um-card-title' style={{padding:'10px 18px 4px'}}>{t('createNewGroup')}</div>
            <div className='um-form-row'>
              <div className='um-form-field'>
                <label>{t('groupName')}</label>
                <input value={groupForm.name} onChange={e => setGroupForm(f => ({...f, name: e.target.value}))} required placeholder={t('groupNamePlaceholder')} />
              </div>
              <div className='um-form-field um-form-action'>
                <label>&nbsp;</label>
                <button type='submit' disabled={groupLoad || !groupForm.name.trim()}>
                  {groupLoad ? t('creating') : t('createGroup')}
                </button>
              </div>
            </div>
          </form>
        </div>

        {/* ── Usuários ── */}
        <div className='um-card'>
          <div className='um-card-title'>{t('users')}</div>
          <table>
            <thead><tr><th>{t('userCol')}</th><th>{t('profileCol')}</th><th>{t('groupCol')}</th><th>{t('actionsCol')}</th></tr></thead>
            <tbody>
              {users.length === 0 && <tr><td colSpan={4} className='empty'>{t('noUsersRegistered')}</td></tr>}
              {users.map(u => {
                const isAdm = u.role === 'administration'
                return (
                  <tr key={u.username}>
                    <td>{u.username}</td>
                    <td>
                      {!isAdm && editRole === u.username ? (
                        <select className='um-role-select' defaultValue={u.role} autoFocus
                          onBlur={e => { if (e.target.value !== u.role) changeUserRole(u.username, e.target.value); else setEditRole(null) }}
                          onChange={e => changeUserRole(u.username, e.target.value)}>
                          <option value='dev'>Dev</option>
                          <option value='reader'>Reader</option>
                        </select>
                      ) : (
                        <span className={`um-role ${roleCls(u.role)}`}
                          title={!isAdm ? t('clickToEditProfile') : ''}
                          style={!isAdm ? {cursor:'pointer'} : {}}
                          onClick={() => !isAdm && setEditRole(u.username)}>
                          {roleLabel(u.role)}
                        </span>
                      )}
                    </td>
                    <td>{u.group_name ? <span className='group-badge'>{u.group_name}</span> : <span style={{opacity:.4,fontSize:11}}>—</span>}</td>
                    <td>
                      <div className='um-actions'>
                        {/* MFA toggle individual apenas para admin */}
                        {isAdm && (
                          <label className={`mfa-switch ${mfaLoading === u.username ? 'mfa-switch-loading' : ''}`} title={t('mfaLabel')}>
                            <input type='checkbox' checked={!!u.totp_enabled}
                              disabled={mfaLoading === u.username} onChange={() => toggleAdminMFA(u)} />
                            <span className='mfa-slider' />
                          </label>
                        )}
                        {/* Resetar MFA: visível quando MFA está habilitado (configurado ou aguardando setup) */}
                        {u.totp_enabled && (
                          <button className='um-btn-mfa-reset' onClick={() => resetUserMFA(u.username)}
                            title={u.totp_configured ? t('forceMfaReconfigure') : t('mfaEnabledWaiting')}>
                            {t('resetMfa')}
                          </button>
                        )}
                        <button className='um-btn-pwd' onClick={() => { setPwdTarget(u.username); setPwdValue(''); setPwdMsg({ type: '', text: '' }) }}>
                          {t('changePassword')}
                        </button>
                        {!isAdm && (
                          <button className='um-btn-del' onClick={() => deleteUser(u.username)}>{t('removeUser')}</button>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        {/* ── Alterar senha ── */}
        <div className='um-card'>
          <div className='um-card-title'>{t('changePasswordTitle')}</div>
          {pwdMsg.text && <div className={`um-msg ${pwdMsg.type}`}>{pwdMsg.text}</div>}
          <form className='um-form' onSubmit={changePassword}>
            <div className='um-form-row'>
              <div className='um-form-field'>
                <label>{t('userCol')}</label>
                <select value={pwdTarget} onChange={e => setPwdTarget(e.target.value)} required>
                  <option value=''>{t('selectUser')}</option>
                  {users.map(u => <option key={u.username} value={u.username}>{u.username}</option>)}
                </select>
              </div>
              <div className='um-form-field'>
                <label>{t('newPassword')}</label>
                <input type='password' value={pwdValue} onChange={e => setPwdValue(e.target.value)} required />
              </div>
              <div className='um-form-field um-form-action'>
                <label>&nbsp;</label>
                <button type='submit' disabled={pwdLoad || !pwdTarget || !pwdValue}>
                  {pwdLoad ? t('savingNewPassword') : t('saveNewPassword')}
                </button>
              </div>
            </div>
          </form>
        </div>

        {/* ── Criar usuário ── */}
        <div className='um-card'>
          <div className='um-card-title'>{t('createUser')}</div>
          {createMsg.text && <div className={`um-msg ${createMsg.type}`}>{createMsg.text}</div>}
          <form className='um-form' onSubmit={createUser}>
            <div className='um-form-row'>
              <div className='um-form-field'>
                <label>{t('userCol')}</label>
                <input value={createForm.username} onChange={e => setCreateForm(f => ({...f, username: e.target.value}))} required />
              </div>
              <div className='um-form-field'>
                <label>{t('password')}</label>
                <input type='password' value={createForm.password} onChange={e => setCreateForm(f => ({...f, password: e.target.value}))} required />
              </div>
              <div className='um-form-field'>
                <label>{t('profileField')}</label>
                <select value={createForm.role} onChange={e => setCreateForm(f => ({...f, role: e.target.value}))}>
                  <option value='administration'>Administration</option>
                  <option value='reader'>Reader</option>
                  <option value='dev'>Dev</option>
                </select>
              </div>
              <div className='um-form-field'>
                <label>{t('groupCol')}</label>
                <select value={createForm.group_name} onChange={e => setCreateForm(f => ({...f, group_name: e.target.value}))}>
                  <option value=''>{t('noGroup')}</option>
                  {groups.map(g => <option key={g.name} value={g.name}>{g.name}</option>)}
                </select>
              </div>
              <div className='um-form-field um-form-action'>
                <label>&nbsp;</label>
                <button type='submit' disabled={createLoad}>
                  {createLoad ? t('creatingUser') : t('createUserBtn')}
                </button>
              </div>
            </div>
          </form>
        </div>

      </div>
    </div>
  )
}

const THEMES = [
  { id: 'dark',       label: 'Dark',               color: '#8b5cf6' },
  { id: 'light',      label: 'Light',              color: '#7c3aed' },
  { id: 'dracula',    label: 'Dracula',             color: '#bd93f9' },
  { id: 'nord',       label: 'Nord',                color: '#88c0d0' },
  { id: 'tokyo',      label: 'Tokyo Night',         color: '#7aa2f7' },
  { id: 'sop',        label: 'Shades of Purple',    color: '#FAD000' },
  { id: 'cyberpunk',  label: 'Cyberpunk 2077',      color: '#fff000' },
  { id: 'tomorrow',   label: 'Tomorrow Night Blue', color: '#64a8ff' },
  { id: 'solarized',  label: 'Solarized Dark',      color: '#268bd2' },
]

// ── Documentação por perfil ──────────────────────────────────────────────────
const HELP_TOPIC_ROLES = {
  monitor:          ['administration', 'reader'],
  top10:            ['administration', 'reader'],
  historico:        ['administration', 'reader'],
  namespaces:       ['administration', 'reader'],
  nodes:            ['administration'],
  storage:          ['administration', 'reader'],
  orphans:          ['administration', 'reader'],
  containers:       ['administration', 'reader'],
  docker:           ['administration', 'reader'],
  helm:             ['administration', 'reader'],
  deployments:      ['administration', 'reader'],
  analysis:         ['administration', 'reader'],
  dashboards:       ['administration', 'reader'],
  logs:             ['administration', 'reader', 'dev'],
  'admin-clusters': ['administration'],
  'admin-users':    ['administration'],
  'dev-access':     ['dev'],
}

function getHelpTopics(lang) {
  const src = translations[lang]?.helpTopics || translations['pt'].helpTopics
  return Object.entries(HELP_TOPIC_ROLES).map(([id, roles]) => ({
    id, roles, ...src[id],
  }))
}

// ── Análise de boas práticas ──────────────────────────────────────────────────

const SEVERITY_COLOR = { critical: '#f87171', warning: '#fbbf24', info: '#60a5fa' }
const SEVERITY_BG    = { critical: 'rgba(248,113,113,0.10)', warning: 'rgba(251,191,36,0.10)', info: 'rgba(96,165,250,0.10)' }

function AnalysisTab({ clusters, lang }) {
  const t = useT(lang || 'pt')
  const CATEGORY_LABEL = t('categoryLabels') || { security: 'Segurança', reliability: 'Confiabilidade', resources: 'Recursos', 'best-practices': 'Boas Práticas' }
  const [data,     setData]     = useState(null)
  const [loading,  setLoading]  = useState(false)
  const [error,    setError]    = useState(null)
  const [filterSev, setFilterSev] = useState('all')
  const [filterCat, setFilterCat] = useState('all')
  const [filterNs,   setFilterNs]   = useState('')
  const [filterNode, setFilterNode] = useState('')
  const [pageSize,  setPageSize]  = useState(50)
  const [page,      setPage]      = useState(1)

  const [anaCluster,   setAnaCluster]   = useState('')
  const [anaNamespace, setAnaNamespace] = useState('')
  const [anaNsList,    setAnaNsList]    = useState([])

  useEffect(() => {
    if (clusters.length > 0 && !anaCluster) setAnaCluster(clusters[0])
  }, [clusters])

  useEffect(() => {
    if (!anaCluster) { setAnaNsList([]); setAnaNamespace(''); return }
    axios.get(`/api/namespaces?cluster=${anaCluster}`)
      .then(r => setAnaNsList(r.data || []))
      .catch(() => setAnaNsList([]))
    setAnaNamespace('')
  }, [anaCluster])

  function runAnalysis() {
    if (!anaCluster) return
    setLoading(true); setError(null); setData(null)
    const params = new URLSearchParams({ cluster: anaCluster })
    if (anaNamespace) params.set('namespace', anaNamespace)
    axios.get(`/api/analysis?${params}`)
      .then(r => setData(r.data))
      .catch(e => setError(e.response?.data || e.message))
      .finally(() => setLoading(false))
  }

  const findings = (data?.findings || []).filter(f => {
    if (filterSev !== 'all' && f.severity !== filterSev) return false
    if (filterCat !== 'all' && f.category !== filterCat) return false
    if (filterNs && f.namespace !== filterNs) return false
    if (filterNode && !(f.resource_kind === 'Node' && f.resource_name === filterNode)) return false
    return true
  })

  const totalPages  = Math.max(1, Math.ceil(findings.length / pageSize))
  const currentPage = Math.min(page, totalPages)
  const paginated   = findings.slice((currentPage - 1) * pageSize, currentPage * pageSize)

  function changeFilter(fn) { fn(); setPage(1) }

  const namespaces = data ? [...new Set(data.findings.map(f => f.namespace))].sort() : []
  const nodes      = data?.nodes || []

  return (
    <div style={{ padding: '1rem' }}>
      {/* Cabeçalho */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.2rem', flexWrap: 'wrap' }}>
        <div>
          <div style={{ fontSize: 18, fontWeight: 700, color: 'var(--text-1)' }}>{t('analysisTitle')}</div>
          <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 2 }}>
            {t('analysisSubtitle')}
          </div>
        </div>
        <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', flexWrap: 'wrap' }}>
          <select value={anaCluster} onChange={e => setAnaCluster(e.target.value)} style={{ minWidth: 140 }}>
            <option value=''>{t('clusterPlaceholder')}</option>
            {clusters.map(c => <option key={c} value={c}>{c}</option>)}
          </select>
          <select value={anaNamespace} onChange={e => setAnaNamespace(e.target.value)} style={{ minWidth: 140 }}>
            <option value=''>{t('allNamespaces')}</option>
            {anaNsList.map(n => <option key={n} value={n}>{n}</option>)}
          </select>
        </div>
        <button onClick={runAnalysis} disabled={loading || !anaCluster}
          style={{
            marginLeft: 'auto',
            display: 'flex', alignItems: 'center', gap: '0.5rem',
            padding: '0.65rem 1.5rem',
            fontSize: 15, fontWeight: 700, letterSpacing: '0.03em',
            borderRadius: 10, border: 'none', cursor: loading || !anaCluster ? 'not-allowed' : 'pointer',
            background: loading || !anaCluster
              ? 'rgba(139,92,246,0.25)'
              : 'linear-gradient(135deg, #7c3aed 0%, #4f46e5 100%)',
            color: loading || !anaCluster ? 'rgba(255,255,255,0.4)' : '#fff',
            boxShadow: loading || !anaCluster ? 'none' : '0 4px 18px rgba(124,58,237,0.45)',
            transition: 'all 0.2s ease',
          }}
          onMouseEnter={e => { if (!loading && anaCluster) e.currentTarget.style.transform = 'translateY(-1px)' }}
          onMouseLeave={e => { e.currentTarget.style.transform = 'translateY(0)' }}
        >
          {loading
            ? <><svg width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2.5'
                style={{ animation: 'spin 1s linear infinite' }}>
                <path d='M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83'/>
              </svg> {t('analyzing')}</>
            : <><svg width='16' height='16' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2.5'>
                <circle cx='11' cy='11' r='8'/><path d='m21 21-4.35-4.35M11 8v6M8 11h6'/>
              </svg> {t('analyzeCluster')}</>
          }
        </button>
      </div>

      {!anaCluster && (
        <div className='empty'>{t('selectClusterForAnalysis')}</div>
      )}

      {error && (
        <div style={{ color: '#f87171', background: 'rgba(248,113,113,0.08)', border: '1px solid rgba(248,113,113,0.2)', borderRadius: 8, padding: '0.75rem 1rem', marginBottom: '1rem' }}>
          {t('analysisError')}: {error}
        </div>
      )}

      {loading && (
        <div className='empty'>{t('collectingData')}</div>
      )}

      {data && !loading && (
        <>
          {/* Cards de resumo */}
          <div style={{ display: 'flex', gap: '0.75rem', marginBottom: '1.2rem', flexWrap: 'wrap' }}>
            {[
              { key: 'critical', label: t('criticals'), val: data.summary.critical  },
              { key: 'warning',  label: t('warnings'),  val: data.summary.warning   },
              { key: 'info',     label: t('tips'),      val: data.summary.info      },
            ].map(s => (
              <div key={s.key} onClick={() => setFilterSev(filterSev === s.key ? 'all' : s.key)}
                style={{
                  flex: '1 1 120px', minWidth: 110, cursor: 'pointer',
                  background: filterSev === s.key ? SEVERITY_BG[s.key] : 'var(--surface)',
                  border: `1px solid ${filterSev === s.key ? SEVERITY_COLOR[s.key] : 'var(--border)'}`,
                  borderRadius: 10, padding: '0.75rem 1rem', textAlign: 'center',
                  transition: 'all 0.15s',
                }}>
                <div style={{ fontSize: 28, fontWeight: 800, color: SEVERITY_COLOR[s.key] }}>{s.val}</div>
                <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: 2 }}>{s.label}</div>
              </div>
            ))}
            <div style={{
              flex: '1 1 180px', background: 'var(--surface)', border: '1px solid var(--border)',
              borderRadius: 10, padding: '0.75rem 1rem', display: 'flex', flexDirection: 'column', justifyContent: 'center',
            }}>
              <div style={{ fontSize: 12, color: 'var(--text-2)' }}>
                {data.summary.scanned_pods} pods · {data.summary.scanned_deployments} deployments · {data.summary.scanned_nodes} nodes · {data.summary.scanned_namespaces} namespaces
              </div>
              <div style={{ fontSize: 11, color: 'var(--text-2)', marginTop: 4 }}>
                {t('analysisDuration')(data.summary.duration_ms)}
              </div>
            </div>
          </div>

          {/* Filtros */}
          <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem', flexWrap: 'wrap', alignItems: 'center' }}>
            <span style={{ fontSize: 12, color: 'var(--text-2)' }}>{t('filterLabel')}</span>
            {['all', 'critical', 'warning', 'info'].map(s => (
              <button key={s} onClick={() => changeFilter(() => setFilterSev(s))}
                style={{
                  fontSize: 11, padding: '3px 10px', borderRadius: 20, border: 'none', cursor: 'pointer',
                  background: filterSev === s ? (s === 'all' ? 'var(--border-focus)' : SEVERITY_COLOR[s]) : 'var(--ctrl-bg)',
                  color: filterSev === s ? '#fff' : 'var(--text-2)',
                  fontWeight: filterSev === s ? 700 : 400,
                }}>
                {s === 'all' ? t('allFilter') : s.charAt(0).toUpperCase() + s.slice(1)}
              </button>
            ))}
            <span style={{ fontSize: 12, color: 'var(--text-2)', marginLeft: 8 }}>{t('categoryLabel')}</span>
            {['all', 'security', 'reliability', 'resources'].map(c => (
              <button key={c} onClick={() => changeFilter(() => setFilterCat(c))}
                style={{
                  fontSize: 11, padding: '3px 10px', borderRadius: 20, border: 'none', cursor: 'pointer',
                  background: filterCat === c ? 'var(--border-focus)' : 'var(--ctrl-bg)',
                  color: filterCat === c ? '#fff' : 'var(--text-2)',
                  fontWeight: filterCat === c ? 700 : 400,
                }}>
                {c === 'all' ? t('allCategories') : CATEGORY_LABEL[c]}
              </button>
            ))}
            {namespaces.length > 1 && (
              <select value={filterNs} onChange={e => changeFilter(() => { setFilterNs(e.target.value); setFilterNode('') })}
                style={{ fontSize: 11, padding: '3px 8px', borderRadius: 6, background: 'var(--input-bg)', color: 'var(--text-1)', border: '1px solid var(--border-input)', marginLeft: 8 }}>
                <option value=''>{t('allNamespacesFilter')}</option>
                {namespaces.map(n => <option key={n} value={n}>{n}</option>)}
              </select>
            )}
            {nodes.length > 0 && (
              <select value={filterNode} onChange={e => changeFilter(() => { setFilterNode(e.target.value); setFilterNs('') })}
                style={{ fontSize: 11, padding: '3px 8px', borderRadius: 6, background: 'var(--input-bg)', color: 'var(--text-1)', border: '1px solid var(--border-input)', marginLeft: 4 }}>
                <option value=''>{t('allNodesFilter')}</option>
                {nodes.map(n => <option key={n} value={n}>{n}</option>)}
              </select>
            )}
          </div>

          {/* Lista de achados */}
          {findings.length === 0 ? (
            <div className='empty'>{t('noFindings')}</div>
          ) : (
            <>
              {/* Barra de paginação superior */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginBottom: '0.75rem', flexWrap: 'wrap' }}>
                <span style={{ fontSize: 12, color: 'var(--text-2)' }}>
                  {t('findings')(findings.length)} · {t('pageOf')(currentPage, totalPages)}
                </span>
                <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-2)' }}>{t('itemsPerPage')}</span>
                  {[50, 100, 200].map(n => (
                    <button key={n} onClick={() => { setPageSize(n); setPage(1) }}
                      style={{
                        fontSize: 12, padding: '3px 10px', borderRadius: 6, border: 'none', cursor: 'pointer',
                        background: pageSize === n ? 'var(--border-focus)' : 'var(--ctrl-bg)',
                        color: pageSize === n ? '#fff' : 'var(--text-2)',
                        fontWeight: pageSize === n ? 700 : 400,
                      }}>{n}</button>
                  ))}
                </div>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                {paginated.map((f, i) => (
                  <div key={i} style={{
                    background: SEVERITY_BG[f.severity],
                    border: `1px solid ${SEVERITY_COLOR[f.severity]}33`,
                    borderLeft: `3px solid ${SEVERITY_COLOR[f.severity]}`,
                    borderRadius: 8, padding: '0.75rem 1rem',
                  }}>
                    <div style={{ display: 'flex', alignItems: 'flex-start', gap: '0.75rem', flexWrap: 'wrap' }}>
                      <span style={{
                        fontSize: 10, fontWeight: 700, padding: '2px 7px', borderRadius: 4,
                        background: SEVERITY_COLOR[f.severity] + '22', color: SEVERITY_COLOR[f.severity],
                        textTransform: 'uppercase', letterSpacing: '0.05em', flexShrink: 0,
                      }}>{f.severity}</span>
                      <span style={{
                        fontSize: 10, padding: '2px 7px', borderRadius: 4,
                        background: 'var(--ctrl-bg)', color: 'var(--text-2)',
                        textTransform: 'uppercase', letterSpacing: '0.05em', flexShrink: 0,
                      }}>{CATEGORY_LABEL[f.category] || f.category}</span>
                      <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-1)', flex: 1 }}>{f.message}</span>
                    </div>
                    <div style={{ display: 'flex', gap: '1rem', marginTop: '0.4rem', flexWrap: 'wrap' }}>
                      <span style={{ fontSize: 11, color: 'var(--text-2)', fontFamily: 'monospace' }}>
                        {f.resource_kind}/{f.resource_name}
                        {f.namespace ? ` · ${f.namespace}` : ''}
                        {f.container ? ` · ${f.container}` : ''}
                      </span>
                    </div>
                    <div style={{ fontSize: 12, color: 'var(--text-2)', marginTop: '0.4rem', borderTop: '1px solid rgba(255,255,255,0.06)', paddingTop: '0.4rem' }}>
                      💡 {f.recommendation}
                    </div>
                  </div>
                ))}
              </div>

              {/* Controles de navegação entre páginas */}
              {totalPages > 1 && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '0.5rem', marginTop: '1rem' }}>
                  <button onClick={() => setPage(1)} disabled={currentPage === 1}
                    style={{ padding: '4px 10px', borderRadius: 6, border: 'none', cursor: currentPage === 1 ? 'not-allowed' : 'pointer', background: 'var(--ctrl-bg)', color: currentPage === 1 ? 'var(--text-2)' : 'var(--text-1)', opacity: currentPage === 1 ? 0.4 : 1 }}>
                    «
                  </button>
                  <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={currentPage === 1}
                    style={{ padding: '4px 10px', borderRadius: 6, border: 'none', cursor: currentPage === 1 ? 'not-allowed' : 'pointer', background: 'var(--ctrl-bg)', color: currentPage === 1 ? 'var(--text-2)' : 'var(--text-1)', opacity: currentPage === 1 ? 0.4 : 1 }}>
                    ‹
                  </button>
                  {Array.from({ length: totalPages }, (_, i) => i + 1)
                    .filter(p => p === 1 || p === totalPages || Math.abs(p - currentPage) <= 2)
                    .reduce((acc, p, idx, arr) => {
                      if (idx > 0 && p - arr[idx - 1] > 1) acc.push('…')
                      acc.push(p)
                      return acc
                    }, [])
                    .map((p, i) => p === '…'
                      ? <span key={`e${i}`} style={{ color: 'var(--text-2)', padding: '0 4px' }}>…</span>
                      : <button key={p} onClick={() => setPage(p)}
                          style={{ padding: '4px 10px', borderRadius: 6, border: 'none', cursor: 'pointer', fontWeight: currentPage === p ? 700 : 400, background: currentPage === p ? 'var(--border-focus)' : 'var(--ctrl-bg)', color: currentPage === p ? '#fff' : 'var(--text-1)' }}>
                          {p}
                        </button>
                    )
                  }
                  <button onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={currentPage === totalPages}
                    style={{ padding: '4px 10px', borderRadius: 6, border: 'none', cursor: currentPage === totalPages ? 'not-allowed' : 'pointer', background: 'var(--ctrl-bg)', color: currentPage === totalPages ? 'var(--text-2)' : 'var(--text-1)', opacity: currentPage === totalPages ? 0.4 : 1 }}>
                    ›
                  </button>
                  <button onClick={() => setPage(totalPages)} disabled={currentPage === totalPages}
                    style={{ padding: '4px 10px', borderRadius: 6, border: 'none', cursor: currentPage === totalPages ? 'not-allowed' : 'pointer', background: 'var(--ctrl-bg)', color: currentPage === totalPages ? 'var(--text-2)' : 'var(--text-1)', opacity: currentPage === totalPages ? 0.4 : 1 }}>
                    »
                  </button>
                </div>
              )}
            </>
          )}
        </>
      )}
    </div>
  )
}

function HelpModal({ role, onClose, lang }) {
  const tl = useT(lang || 'pt')
  const topics = getHelpTopics(lang || 'pt').filter(t => t.roles.includes(role))
  const [active, setActive] = useState(topics[0]?.id || '')
  const topic = topics.find(t => t.id === active)

  return (
    <div className='help-overlay' onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className='help-modal'>
        <div className='help-header'>
          <span className='help-title'>
            <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2' style={{width:18,height:18,marginRight:8,verticalAlign:'middle'}}>
              <circle cx='12' cy='12' r='10'/><path d='M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3'/><line x1='12' y1='17' x2='12.01' y2='17' strokeWidth='3' strokeLinecap='round'/>
            </svg>
            {tl('documentation')}
          </span>
          <button className='help-close' onClick={onClose}>✕</button>
        </div>
        <div className='help-body'>
          <nav className='help-nav'>
            {topics.map(t => (
              <button key={t.id} className={`help-nav-item ${active === t.id ? 'active' : ''}`}
                onClick={() => setActive(t.id)}>
                {t.title}
              </button>
            ))}
          </nav>
          <div className='help-content'>
            {topic && (
              <>
                <h2 className='help-topic-title'>{topic.title}</h2>
                {topic.content.map((c, i) => (
                  <div key={i} className='help-section'>
                    <div className='help-section-title'>{c.h}</div>
                    <p className='help-section-body'>{c.p}</p>
                  </div>
                ))}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

const WIDGET_CATALOG_TYPES = [
  { type: 'pods-summary',     w: 4, h: 3 },
  { type: 'nodes-summary',    w: 4, h: 3 },
  { type: 'top-cpu',          w: 6, h: 4 },
  { type: 'top-mem',          w: 6, h: 4 },
  { type: 'alerts',           w: 3, h: 3 },
  { type: 'helm-summary',     w: 3, h: 3 },
  { type: 'docker-summary',   w: 3, h: 3 },
  { type: 'pods-pie',         w: 4, h: 4 },
  { type: 'pods-not-running', w: 6, h: 4 },
  { type: 'helm-pie',         w: 4, h: 4 },
  { type: 'cpu-bar',          w: 6, h: 4 },
  { type: 'mem-bar',          w: 6, h: 4 },
  { type: 'cpu-line',         w: 8, h: 4 },
  { type: 'mem-line',         w: 8, h: 4 },
]

const WIDGET_ICONS = {
  'pods-summary':   '⬡',
  'nodes-summary':  '◈',
  'top-cpu':        '⚡',
  'top-mem':        '◉',
  'alerts':         '⚠',
  'helm-summary':   '⎈',
  'docker-summary': '⬛',
  'pods-pie':         '◔',
  'pods-not-running': '⊘',
  'helm-pie':   '◕',
  'cpu-bar':    '▬',
  'mem-bar':    '▭',
  'cpu-line':   '〜',
  'mem-line':   '〰',
}

// ── Cores por status de pod ───────────────────────────────────────────────────
function podStatusColor(s) {
  const l = (s || '').toLowerCase()
  if (l === 'running')                                    return '#34d399'
  if (l === 'pending' || l === 'containercreating')       return '#fbbf24'
  if (l === 'succeeded')                                  return '#60a5fa'
  if (l === 'failed' || l.includes('error'))              return '#f87171'
  if (l.includes('backoff') || l.includes('crashloop'))   return '#fb923c'
  if (l === 'unknown')                                    return '#6b7280'
  return '#a78bfa'  // outros waiting reasons
}

// ── Tooltip escuro universal (não usa CSS vars para evitar branco no tema light) ──
const TT_CONTENT = {
  background: '#161622', border: '1px solid rgba(139,92,246,0.35)',
  borderRadius: 6, fontSize: 12, color: '#e2e8f0',
  boxShadow: '0 4px 16px rgba(0,0,0,0.55)', padding: '6px 10px',
}
const TT_ITEM = { color: '#c4b5fd' }
const TT_LABEL = { color: '#818cf8', fontWeight: 600, marginBottom: 2 }
// Cursor: substitui o fundo branco padrão do Recharts no hover
const TT_CURSOR = { fill: 'rgba(139,92,246,0.07)' }

// ── Speedometer gauge estilo Grafana ─────────────────────────────────────────
// Coordenadas fixas (viewBox 200×170), responsivo via width/height 100%
// Arco de 220° de 200° até 60° (sentido horário a partir do topo)
const SG_VBW = 200, SG_VBH = 170
const SG_CX = 100, SG_CY = 95
const SG_R  = 68
const SG_SW = 14        // espessura do arco
const SG_START = 200    // grau inicial
const SG_SWEEP = 220    // amplitude total

function sgPolar(deg, r = SG_R) {
  const rad = (deg - 90) * Math.PI / 180
  return { x: SG_CX + r * Math.cos(rad), y: SG_CY + r * Math.sin(rad) }
}

function sgArc(startDeg, sweepDeg) {
  const s = sgPolar(startDeg)
  const e = sgPolar(startDeg + sweepDeg)
  const large = sweepDeg > 180 ? 1 : 0
  return `M ${s.x.toFixed(2)} ${s.y.toFixed(2)} A ${SG_R} ${SG_R} 0 ${large} 1 ${e.x.toFixed(2)} ${e.y.toFixed(2)}`
}

const SG_ZONES = [
  { from: 0,    to: 0.75, color: '#37872d' },
  { from: 0.75, to: 0.90, color: '#e0b400' },
  { from: 0.90, to: 1.00, color: '#c4162a' },
]

function SpeedGauge({ pct, value, label }) {
  const p = Math.min(Math.max(pct || 0, 0), 1)
  const color = p >= 0.90 ? '#f87171' : p >= 0.75 ? '#fbbf24' : '#34d399'

  return (
    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
      <svg viewBox={`0 0 ${SG_VBW} ${SG_VBH}`}
        width='100%' height='100%'
        style={{ display: 'block', overflow: 'visible' }}>

        {/* trilha por zonas */}
        {SG_ZONES.map((z, i) => (
          <path key={i}
            d={sgArc(SG_START + z.from * SG_SWEEP, (z.to - z.from) * SG_SWEEP)}
            fill='none' stroke={z.color} strokeWidth={SG_SW}
            strokeLinecap='butt' opacity={0.3} />
        ))}

        {/* arco preenchido */}
        {p > 0 && (
          <path d={sgArc(SG_START, p * SG_SWEEP)}
            fill='none' stroke={color} strokeWidth={SG_SW}
            strokeLinecap='round'
            style={{ filter: `drop-shadow(0 0 5px ${color}99)`, transition: 'all 0.5s ease' }} />
        )}
      </svg>

      {/* valor e label sobrepostos em HTML para suportar JSX */}
      <div style={{
        position: 'absolute', inset: 0,
        display: 'flex', flexDirection: 'column',
        alignItems: 'center', justifyContent: 'center',
        paddingBottom: '4%', pointerEvents: 'none',
      }}>
        <span style={{
          fontSize: 'clamp(34px, 13cqh, 72px)', fontWeight: 800,
          color, lineHeight: 1, pointerEvents: 'auto',
          textAlign: 'center',
        }}>
          {value}
        </span>
        {label && (
          <span style={{
            fontSize: 'clamp(12px, 2.8cqh, 18px)', marginTop: 6,
            color: 'rgba(255,255,255,0.38)',
            textTransform: 'uppercase', letterSpacing: '0.05em',
            textAlign: 'center',
          }}>
            {label}
          </span>
        )}
      </div>
    </div>
  )
}

// ── Ícones SVG dos widgets ────────────────────────────────────────────────────
const DW_ICONS = {
  'pods-summary':  <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><rect x='2' y='7' width='20' height='14' rx='2'/><path d='M16 7V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v2'/></svg>,
  'nodes-summary': <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><circle cx='12' cy='12' r='3'/><path d='M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83'/></svg>,
  'alerts':        <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><path d='M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z'/><line x1='12' y1='9' x2='12' y2='13'/><line x1='12' y1='17' x2='12.01' y2='17' strokeWidth='3' strokeLinecap='round'/></svg>,
  'helm-summary':  <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><circle cx='12' cy='12' r='10'/><circle cx='12' cy='12' r='3'/><line x1='12' y1='2' x2='12' y2='9'/><line x1='12' y1='15' x2='12' y2='22'/><line x1='2' y1='12' x2='9' y2='12'/><line x1='15' y1='12' x2='22' y2='12'/></svg>,
  'docker-summary':<svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><rect x='2' y='2' width='20' height='20' rx='3'/><path d='M9 9h6M9 12h6M9 15h4'/></svg>,
  'top-cpu':       <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><polyline points='22 12 18 12 15 21 9 3 6 12 2 12'/></svg>,
  'top-mem':       <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><ellipse cx='12' cy='5' rx='9' ry='3'/><path d='M21 12c0 1.66-4 3-9 3s-9-1.34-9-3'/><path d='M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5'/></svg>,
  'pods-pie':          <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><path d='M21.21 15.89A10 10 0 1 1 8 2.83'/><path d='M22 12A10 10 0 0 0 12 2v10z'/></svg>,
  'helm-pie':          <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><path d='M21.21 15.89A10 10 0 1 1 8 2.83'/><path d='M22 12A10 10 0 0 0 12 2v10z'/></svg>,
  'pods-not-running':  <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><circle cx='12' cy='12' r='10'/><line x1='4.93' y1='4.93' x2='19.07' y2='19.07'/></svg>,
  'cpu-bar':       <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><line x1='18' y1='20' x2='18' y2='10'/><line x1='12' y1='20' x2='12' y2='4'/><line x1='6' y1='20' x2='6' y2='14'/></svg>,
  'mem-bar':       <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><line x1='18' y1='20' x2='18' y2='10'/><line x1='12' y1='20' x2='12' y2='4'/><line x1='6' y1='20' x2='6' y2='14'/></svg>,
  'cpu-line':      <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><polyline points='22 12 18 12 15 21 9 3 6 12 2 12'/></svg>,
  'mem-line':      <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'><polyline points='2 20 6 14 10 17 14 8 18 11 22 4'/></svg>,
}

function PanelHeader({ type, title }) {
  return (
    <div className='dw-panel-header'>
      <span className='dw-panel-icon'>{DW_ICONS[type]}</span>
      <span className='dw-panel-title'>{title}</span>
    </div>
  )
}

function ProgBar({ pct }) {
  const cls = pct >= 90 ? 'dw-prog-alert' : pct >= 75 ? 'dw-prog-warn' : 'dw-prog-ok'
  return (
    <div className={`dw-prog-wrap ${cls}`}>
      <div className='dw-prog-track'>
        <div className='dw-prog-fill' style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
      <span className='dw-prog-label'>{pct}%</span>
    </div>
  )
}

// ── StatHover: número clicável que exibe lista de nomes em popover ───────────
function StatHover({ children, items, color }) {
  const [open, setOpen] = useState(false)
  const ref = useRef(null)
  useEffect(() => {
    if (!open) return
    function handler(e) { if (ref.current && !ref.current.contains(e.target)) setOpen(false) }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])
  if (!items || items.length === 0) return <>{children}</>
  return (
    <span ref={ref} style={{ position: 'relative', display: 'inline-flex', alignItems: 'center' }}>
      <span
        style={{ cursor: 'pointer', textDecoration: 'underline dotted', textUnderlineOffset: 3 }}
        onClick={() => setOpen(o => !o)}>
        {children}
      </span>
      {open && (
        <div style={{
          position: 'absolute', bottom: '110%', left: '50%', transform: 'translateX(-50%)',
          background: '#161622', border: '1px solid rgba(139,92,246,0.35)',
          borderRadius: 8, padding: '8px 12px', zIndex: 9999,
          boxShadow: '0 8px 32px rgba(0,0,0,0.7)', minWidth: 200, maxWidth: 320,
          maxHeight: 260, overflowY: 'auto',
        }}>
          <div style={{ fontSize: 10, color: '#818cf8', fontWeight: 700, marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            {items.length} {items.length === 1 ? 'item' : 'items'}
          </div>
          {items.map((item, i) => (
            <div key={i} style={{
              fontSize: 11, color: color || '#e2e8f0', padding: '2px 0',
              borderBottom: i < items.length - 1 ? '1px solid rgba(255,255,255,0.06)' : 'none',
              fontFamily: 'monospace', wordBreak: 'break-all',
            }}>
              {item}
            </div>
          ))}
        </div>
      )}
    </span>
  )
}

// ── Paleta de cores para séries por pod ──────────────────────────────────────
const SERIES_COLORS = [
  '#ff9830','#73bf69','#6ed0e0','#ef843c','#e24d42',
  '#1f78c1','#ba43a9','#705da0','#508642','#cca300',
  '#f9934e','#44a896','#e0752d','#614d93','#0a437c',
]

// ── Widget de série temporal (busca próprio dado) ─────────────────────────────
function fmtTsLabel(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString('pt-BR', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function addLabels(pts) {
  return (pts || []).map(p => ({ ...p, label: fmtTsLabel(p.ts) }))
}

// janela de tempo exibida
const ZOOM_OPTS = [
  { label: '1h',  ms:      3600_000 },
  { label: '6h',  ms: 6  * 3600_000 },
  { label: '12h', ms: 12 * 3600_000 },
  { label: '24h', ms: 24 * 3600_000 },
  { label: '48h', ms: 48 * 3600_000 },
]

// granularidade dos ticks do eixo X (duplo clique = null = auto)
const TICK_ZOOMS = [
  { label: '5m',  min: 5  },
  { label: '10m', min: 10 },
  { label: '30m', min: 30 },
]

function applyZoom(pts, zoomMs) {
  if (!pts || !pts.length || !zoomMs) return pts
  const cutoff = Date.now() - zoomMs
  return pts.filter(p => p.ts && new Date(p.ts).getTime() >= cutoff)
}

function fmtTick(iso, zoomMs) {
  if (!iso) return ''
  const d = new Date(iso)
  if (zoomMs <= 3600_000)
    return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  if (zoomMs <= 24 * 3600_000)
    return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
  return d.toLocaleString('pt-BR', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// calcula array de timestamps a serem marcados dado intervalo em minutos
function calcTickValues(pts, intervalMin) {
  if (!pts || pts.length <= 1 || !intervalMin) return undefined
  const intervalMs = intervalMin * 60_000
  const ticks = []
  let lastTs = -Infinity
  for (const p of pts) {
    const t = new Date(p.ts).getTime()
    if (t - lastTs >= intervalMs) { ticks.push(p.ts); lastTs = t }
  }
  return ticks
}

// auto: ~6 ticks distribuídos
function autoTickInterval(pts) {
  if (!pts || pts.length <= 1) return 0
  return Math.max(1, Math.floor(pts.length / 6))
}

function TimeSeriesWidget({ type, cluster, refreshTick, lang }) {
  const t = useT(lang || 'pt')
  const [data,     setData]     = useState(null)
  const [loading,  setLoading]  = useState(true)
  const [zoomMs,   setZoomMs]   = useState(3600_000)   // janela de tempo: 1h por padrão
  const [tickZoom, setTickZoom] = useState(null)        // granularidade de ticks: null = auto
  const [mode,     setMode]     = useState('total')
  const [selPods,  setSelPods]  = useState([])

  // busca sempre as últimas 48h — zoom é só filtro de exibição
  const fetchHours = 48

  const isCPU  = type === 'cpu-line'
  const title  = isCPU ? 'CPU' : 'Memória'
  const yFmt   = v => isCPU
    ? (v >= 1000 ? `${(v/1000).toFixed(1)}c` : `${Math.round(v)}m`)
    : (v >= 1024 ? `${(v/1024).toFixed(1)}Gi` : `${v}Mi`)

  useEffect(() => {
    let cancelled = false
    function doFetch() {
      setLoading(true)
      axios.get('/api/dashboard/timeseries', { params: { cluster, hours: fetchHours } })
        .then(r => {
          if (cancelled) return
          const d = r.data || {}
          const processed = {
            totalPoints: addLabels(d.points),
            pods:        d.pods || [],
            podCPU:      addLabels(d.pod_cpu),
            podMem:      addLabels(d.pod_mem),
          }
          setData(processed)
          setSelPods(prev => prev.length ? prev.filter(p => processed.pods.includes(p)) : processed.pods.slice(0, 5))
        })
        .catch(() => { if (!cancelled) setData({ totalPoints: [], pods: [], podCPU: [], podMem: [] }) })
        .finally(() => { if (!cancelled) setLoading(false) })
    }
    doFetch()
    const timer = setInterval(doFetch, 300_000) // auto-refresh a cada 5min
    return () => { cancelled = true; clearInterval(timer) }
  }, [cluster, refreshTick])

  function togglePod(pod) {
    setSelPods(prev => prev.includes(pod) ? prev.filter(p => p !== pod) : [...prev, pod])
  }

  const totalColor = isCPU ? '#ff9830' : '#73bf69'
  const gradId     = isCPU ? `cpuGrad_${type}` : `memGrad_${type}`

  const rawData    = mode === 'total' ? data?.totalPoints : (isCPU ? data?.podCPU : data?.podMem)
  const chartData  = applyZoom(rawData, zoomMs)
  const activePods = (data?.pods || []).filter(p => selPods.includes(p))
  const hasData    = chartData && chartData.length > 0
  const xTicks     = tickZoom ? calcTickValues(chartData, tickZoom) : undefined
  const xInterval  = tickZoom ? 0 : autoTickInterval(chartData)

  return (
    <div className='dw'>
      {/* Header */}
      <div className='dw-panel-header'>
        <span className='dw-panel-icon'>{DW_ICONS[type]}</span>
        <span className='dw-panel-title'>{title}</span>
        <div className='dw-ts-controls'>
          <div className='dw-ts-mode'>
            <button className={`dw-ts-btn ${mode === 'total' ? 'active' : ''}`} onClick={() => setMode('total')}>Total</button>
            <button className={`dw-ts-btn ${mode === 'pods'  ? 'active' : ''}`} onClick={() => setMode('pods')}>Pods</button>
          </div>
          <div className='dw-ts-sep' />
          <div className='dw-ts-hours'>
            {ZOOM_OPTS.map(z => (
              <button key={z.ms} className={`dw-ts-btn ${zoomMs === z.ms ? 'active' : ''}`}
                onClick={() => setZoomMs(z.ms)}>{z.label}</button>
            ))}
          </div>
          <div className='dw-ts-sep' />
          <div className='dw-ts-tickzoom' title={t('tickGranularity')}>
            {TICK_ZOOMS.map(z => (
              <button key={z.min} className={`dw-ts-btn ${tickZoom === z.min ? 'active tick-active' : ''}`}
                onClick={() => setTickZoom(prev => prev === z.min ? null : z.min)}
                onDoubleClick={() => setTickZoom(null)}>
                {z.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Seletor de pods */}
      {mode === 'pods' && data?.pods?.length > 0 && (
        <div className='dw-pod-selector'>
          {data.pods.map((pod, i) => {
            const color = SERIES_COLORS[i % SERIES_COLORS.length]
            const active = selPods.includes(pod)
            return (
              <button key={pod}
                className={`dw-pod-chip ${active ? 'active' : ''}`}
                style={{ '--chip-color': color }}
                onClick={() => togglePod(pod)}
                title={pod}>
                <span className='dw-pod-chip-dot' />
                {pod.length > 22 ? pod.slice(0, 20) + '…' : pod}
              </button>
            )
          })}
        </div>
      )}

      {/* Gráfico */}
      <div className='dw-body' style={{ flex: 1 }}>
        {loading && <div className='dw-loading'>{t('loadingTs')}</div>}
        {!loading && !hasData && (
          <div className='dw-loading'>{t('noDataTs')}</div>
        )}
        {!loading && hasData && (
          <div className='dw-chart' onDoubleClick={() => setTickZoom(null)} title={t('doubleClickReset')}>
            <ResponsiveContainer width='100%' height='100%'>
              <AreaChart data={chartData} margin={{ left: 8, right: 14, top: 6, bottom: 4 }}>
                <defs>
                  {mode === 'total' && (
                    <linearGradient id={gradId} x1='0' y1='0' x2='0' y2='1'>
                      <stop offset='5%'  stopColor={totalColor} stopOpacity={0.28} />
                      <stop offset='95%' stopColor={totalColor} stopOpacity={0.03} />
                    </linearGradient>
                  )}
                  {mode === 'pods' && activePods.map((pod, i) => {
                    const c = SERIES_COLORS[(data.pods.indexOf(pod)) % SERIES_COLORS.length]
                    return (
                      <linearGradient key={pod} id={`grad_${i}`} x1='0' y1='0' x2='0' y2='1'>
                        <stop offset='5%'  stopColor={c} stopOpacity={0.2} />
                        <stop offset='95%' stopColor={c} stopOpacity={0.01} />
                      </linearGradient>
                    )
                  })}
                </defs>
                <CartesianGrid strokeDasharray='3 3' stroke='rgba(255,255,255,0.06)' vertical={false} />
                <XAxis dataKey='ts'
                  tickFormatter={ts => fmtTick(ts, zoomMs)}
                  ticks={xTicks}
                  interval={xTicks ? 0 : xInterval}
                  tick={{ fontSize: 10, fill: 'rgba(255,255,255,0.3)' }}
                  axisLine={false} tickLine={false} />
                <YAxis tickFormatter={yFmt} width={54}
                  tick={{ fontSize: 10, fill: 'rgba(255,255,255,0.3)' }}
                  axisLine={false} tickLine={false} />
                <Tooltip contentStyle={TT_CONTENT} labelStyle={TT_LABEL}
                  cursor={{ stroke: 'rgba(255,255,255,0.15)', strokeWidth: 1, strokeDasharray: '4 4' }}
                  labelFormatter={ts => fmtTick(ts, zoomMs)}
                  formatter={(v, name) => [yFmt(v), name === (isCPU ? 'cpu_m' : 'mem_mi') ? 'Total' : name]} />

                {mode === 'total' && (
                  <Area type='monotone' dataKey={isCPU ? 'cpu_m' : 'mem_mi'}
                    stroke={totalColor} strokeWidth={2}
                    fill={`url(#${gradId})`}
                    dot={false} activeDot={{ r: 4, fill: totalColor, strokeWidth: 0 }} />
                )}

                {mode === 'pods' && activePods.map((pod, i) => {
                  const podIdx = data.pods.indexOf(pod)
                  const c = SERIES_COLORS[podIdx % SERIES_COLORS.length]
                  return (
                    <Area key={pod} type='monotone' dataKey={pod}
                      stroke={c} strokeWidth={1.5}
                      fill={`url(#grad_${i})`}
                      dot={false} activeDot={{ r: 3, fill: c, strokeWidth: 0 }} />
                  )
                })}

              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>
    </div>
  )
}

function DashWidget({ type, summary, cluster, refreshTick, lang }) {
  const t = useT(lang || 'pt')
  if (!summary) return <div className='dw-loading'>–</div>

  if (type === 'pods-summary') {
    const statuses   = summary.pods?.statuses    || {}
    const statusPods = summary.pods?.status_pods || {}
    const total   = Object.values(statuses).reduce((a, b) => a + b, 0)
    const running = statuses['Running'] || 0
    const runPct  = total > 0 ? running / total : 0

    const sorted = Object.entries(statuses).sort(([a], [b]) => {
      if (a === 'Running') return -1
      if (b === 'Running') return 1
      return a.localeCompare(b)
    })

    return (
      <div className='dw'>
        <PanelHeader type={type} title='Pods' />
        <div className='dw-gauge-wrap'>
          <SpeedGauge pct={runPct} value={
            <StatHover items={statusPods['Running']} color='#34d399'>{running}</StatHover>
          } label={`${Math.round(runPct * 100)}% running`} />
        </div>
        <div className='dw-secondary-row' style={{ flexWrap: 'wrap' }}>
          {sorted.filter(([s]) => s !== 'Running').map(([s, n]) => (
            <div key={s} className='dw-secondary-item'>
              <span className='dw-secondary-num' style={{ color: podStatusColor(s) }}>
                <StatHover items={statusPods[s]} color={podStatusColor(s)}>{n}</StatHover>
              </span>
              <span className='dw-secondary-lbl'>{s}</span>
            </div>
          ))}
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num muted'>{total}</span>
            <span className='dw-secondary-lbl'>Total</span>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'nodes-summary') {
    const { ready, not_ready, total, not_ready_names } = summary.nodes
    const readyPct = total > 0 ? ready / total : 0
    return (
      <div className='dw'>
        <PanelHeader type={type} title='Nodes' />
        <div className='dw-gauge-wrap'>
          <SpeedGauge pct={readyPct} value={ready} label={`${Math.round(readyPct * 100)}% ready`} />
        </div>
        <div className='dw-secondary-row'>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num err'>
              <StatHover items={not_ready_names} color='#f87171'>{not_ready}</StatHover>
            </span>
            <span className='dw-secondary-lbl'>Not Ready</span>
          </div>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num muted'>{total}</span>
            <span className='dw-secondary-lbl'>Total</span>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'alerts') {
    const alerts    = summary.alerts || {}
    const alertPods = summary.alert_pods || []
    const critical  = alerts.critical || 0
    const warning   = alerts.warning  || 0
    const total     = critical + warning

    const maxRef = 20
    const pct    = total / maxRef
    const color  = total === 0 ? '#34d399' : critical > 0 ? '#f87171' : '#fbbf24'
    const label  = total === 0 ? t('allNormal') : t('totalAlerts')(total)

    const critItems = alertPods.filter(a => a.severity === 'critical')
      .map(a => `${a.namespace}/${a.pod} [${a.container}] — ${a.reason}`)
    const warnItems = alertPods.filter(a => a.severity === 'warning')
      .map(a => `${a.namespace}/${a.pod} [${a.container}] — ${a.reason}`)

    return (
      <div className='dw'>
        <PanelHeader type={type} title={t('widgetCatalog')['alerts']} />
        <div className='dw-gauge-wrap'>
          <SpeedGauge pct={pct} value={
            <StatHover items={[...critItems, ...warnItems]} color={color}>{total}</StatHover>
          } label={label} />
        </div>
        <div className='dw-secondary-row'>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num' style={{ color: '#f87171' }}>
              <StatHover items={critItems} color='#f87171'>{critical}</StatHover>
            </span>
            <span className='dw-secondary-lbl'>Critical</span>
          </div>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num' style={{ color: '#fbbf24' }}>
              <StatHover items={warnItems} color='#fbbf24'>{warning}</StatHover>
            </span>
            <span className='dw-secondary-lbl'>Warning</span>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'helm-summary') {
    const { deployed, failed, total } = summary.helm
    const deployedPct = total > 0 ? Math.round(deployed / total * 100) : 0
    return (
      <div className='dw'>
        <PanelHeader type={type} title='Helm Releases' />
        <div className='dw-bigstat-wrap'>
          <span className='dw-bigstat-lbl'>Deployed</span>
          <span className='dw-bigstat-num ok'>{deployed}</span>
          <div className='dw-bigstat-bar'>
            <div className='dw-bigstat-bar-fill' style={{ width: `${deployedPct}%`, background: 'var(--success)' }} />
          </div>
          <span className='dw-bigstat-pct'>{t('pctOfTotal')(deployedPct)}</span>
        </div>
        <div className='dw-secondary-row'>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num err'>{failed}</span>
            <span className='dw-secondary-lbl'>Failed</span>
          </div>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num muted'>{total}</span>
            <span className='dw-secondary-lbl'>Total</span>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'docker-summary') {
    const { hosts, running } = summary.docker
    return (
      <div className='dw'>
        <PanelHeader type={type} title='Docker / Podman' />
        <div className='dw-bigstat-wrap'>
          <span className='dw-bigstat-lbl'>Running</span>
          <span className='dw-bigstat-num ok'>{running}</span>
        </div>
        <div className='dw-secondary-row'>
          <div className='dw-secondary-item'>
            <span className='dw-secondary-num muted'>{hosts}</span>
            <span className='dw-secondary-lbl'>Hosts</span>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'top-cpu') {
    return (
      <div className='dw dw-table'>
        <PanelHeader type={type} title={t('topCpuTitle')} />
        <div className='dw-body' style={{ padding: '4px 0 0', overflow: 'hidden' }}>
          <table className='dw-tbl'>
            <thead><tr><th>Container</th><th>Pod</th><th className='dw-tbl-num'>{t('thUsage')}</th><th className='dw-prog-cell'>{t('thUtilization')}</th></tr></thead>
            <tbody>
              {(summary.top_cpu || []).map((c, i) => (
                <tr key={i}>
                  <td>{c.name}</td>
                  <td className='dw-muted'>{c.pod}</td>
                  <td className='dw-tbl-num'>{c.cpu_usage}</td>
                  <td className='dw-prog-cell'><ProgBar pct={c.pct} /></td>
                </tr>
              ))}
              {!summary.top_cpu?.length && <tr><td colSpan={4} className='dw-muted' style={{textAlign:'center',padding:'12px'}}>{t('noData')}</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    )
  }

  if (type === 'top-mem') {
    return (
      <div className='dw dw-table'>
        <PanelHeader type={type} title={t('topMemTitle')} />
        <div className='dw-body' style={{ padding: '4px 0 0', overflow: 'hidden' }}>
          <table className='dw-tbl'>
            <thead><tr><th>Container</th><th>Pod</th><th className='dw-tbl-num'>{t('thUsage')}</th><th className='dw-prog-cell'>{t('thUtilization')}</th></tr></thead>
            <tbody>
              {(summary.top_mem || []).map((c, i) => (
                <tr key={i}>
                  <td>{c.name}</td>
                  <td className='dw-muted'>{c.pod}</td>
                  <td className='dw-tbl-num'>{c.mem_usage}</td>
                  <td className='dw-prog-cell'><ProgBar pct={c.pct} /></td>
                </tr>
              ))}
              {!summary.top_mem?.length && <tr><td colSpan={4} className='dw-muted' style={{textAlign:'center',padding:'12px'}}>{t('noData')}</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    )
  }

  if (type === 'pods-pie') {
    const statuses = summary.pods?.statuses || {}
    const data = Object.entries(statuses)
      .filter(([, v]) => v > 0)
      .map(([name, value]) => ({ name, value, color: podStatusColor(name) }))
    const total = data.reduce((a, d) => a + d.value, 0)
    return (
      <div className='dw'>
        <PanelHeader type={type} title='Pods por Fase' />
        <div className='dw-body' style={{ flex: 1 }}>
          <div className='dw-chart'>
            <ResponsiveContainer width='100%' height='100%'>
              <PieChart>
                <Pie data={data} cx='50%' cy='50%' innerRadius='40%' outerRadius='65%'
                  dataKey='value' paddingAngle={2}>
                  {data.map((d, i) => <Cell key={i} fill={d.color} stroke='rgba(0,0,0,0.3)' strokeWidth={1} />)}
                </Pie>
                <Tooltip contentStyle={TT_CONTENT} itemStyle={TT_ITEM} labelStyle={TT_LABEL}
                  cursor={TT_CURSOR}
                  formatter={(v, n) => [`${v} (${Math.round(v/total*100)}%)`, n]} />
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className='dw-legend'>
            {data.map((d, i) => (
              <span key={i} className='dw-legend-item'>
                <span className='dw-legend-dot' style={{ background: d.color }} />
                {d.name}: <strong style={{ color: d.color }}>{d.value}</strong>
              </span>
            ))}
          </div>
        </div>
      </div>
    )
  }

  if (type === 'helm-pie') {
    const { deployed, failed, total } = summary.helm
    const other = total - deployed - failed
    const data = [
      { name: 'Deployed', value: deployed,       color: '#34d399' },
      { name: 'Failed',   value: failed,          color: '#f87171' },
      { name: 'Outros',   value: other > 0 ? other : 0, color: '#4b5563' },
    ].filter(d => d.value > 0)
    return (
      <div className='dw'>
        <PanelHeader type={type} title='Helm Releases' />
        {total === 0 ? (
          <div className='dw-loading'>{t('noReleases')}</div>
        ) : (
          <div className='dw-body' style={{ flex: 1 }}>
            <div className='dw-chart'>
              <ResponsiveContainer width='100%' height='100%'>
                <PieChart>
                  <Pie data={data} cx='50%' cy='50%' innerRadius='40%' outerRadius='65%'
                    dataKey='value' paddingAngle={2}>
                    {data.map((d, i) => <Cell key={i} fill={d.color} stroke='rgba(0,0,0,0.3)' strokeWidth={1} />)}
                  </Pie>
                  <Tooltip contentStyle={TT_CONTENT} itemStyle={TT_ITEM} labelStyle={TT_LABEL}
                    cursor={TT_CURSOR}
                    formatter={(v, n) => [`${v} (${Math.round(v/total*100)}%)`, n]} />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className='dw-legend'>
              {data.map((d, i) => (
                <span key={i} className='dw-legend-item'>
                  <span className='dw-legend-dot' style={{ background: d.color }} />
                  {d.name}: <strong style={{ color: d.color }}>{d.value}</strong>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    )
  }

  if (type === 'cpu-bar') {
    const items = (summary.top_cpu || []).slice(0, 8).map(c => ({
      name: c.name.length > 16 ? c.name.slice(0, 16) + '…' : c.name,
      pct: c.pct,
      label: c.cpu_usage,
    }))
    return (
      <div className='dw dw-table'>
        <PanelHeader type={type} title={t('topCpuTitle')} />
        <div className='dw-body' style={{ flex: 1 }}>
          <div className='dw-chart'>
            <ResponsiveContainer width='100%' height='100%'>
              <BarChart data={items} layout='vertical' margin={{ left: 0, right: 32, top: 2, bottom: 2 }}>
                <CartesianGrid horizontal={false} strokeDasharray='3 3' stroke='rgba(255,255,255,0.05)' />
                <XAxis type='number' domain={[0, 100]} tickFormatter={v => `${v}%`}
                  tick={{ fontSize: 10, fill: 'rgba(255,255,255,0.3)' }} axisLine={false} tickLine={false} />
                <YAxis type='category' dataKey='name' width={100}
                  tick={{ fontSize: 11, fill: 'rgba(255,255,255,0.55)' }} axisLine={false} tickLine={false} />
                <Tooltip contentStyle={TT_CONTENT} itemStyle={TT_ITEM} labelStyle={TT_LABEL}
                  cursor={TT_CURSOR}
                  formatter={(v, _, props) => [`${v}%  ·  ${props.payload.label}`, 'CPU']} />
                <Bar dataKey='pct' radius={[0, 3, 3, 0]} maxBarSize={14} background={{ fill: 'rgba(255,255,255,0.03)', radius: 3 }}>
                  {items.map((d, i) => (
                    <Cell key={i} fill={d.pct >= 90 ? '#f87171' : d.pct >= 75 ? '#fbbf24' : '#818cf8'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'mem-bar') {
    const items = (summary.top_mem || []).slice(0, 8).map(c => ({
      name: c.name.length > 16 ? c.name.slice(0, 16) + '…' : c.name,
      pct: c.pct,
      label: c.mem_usage,
    }))
    return (
      <div className='dw dw-table'>
        <PanelHeader type={type} title={t('topMemTitle')} />
        <div className='dw-body' style={{ flex: 1 }}>
          <div className='dw-chart'>
            <ResponsiveContainer width='100%' height='100%'>
              <BarChart data={items} layout='vertical' margin={{ left: 0, right: 32, top: 2, bottom: 2 }}>
                <CartesianGrid horizontal={false} strokeDasharray='3 3' stroke='rgba(255,255,255,0.05)' />
                <XAxis type='number' domain={[0, 100]} tickFormatter={v => `${v}%`}
                  tick={{ fontSize: 10, fill: 'rgba(255,255,255,0.3)' }} axisLine={false} tickLine={false} />
                <YAxis type='category' dataKey='name' width={100}
                  tick={{ fontSize: 11, fill: 'rgba(255,255,255,0.55)' }} axisLine={false} tickLine={false} />
                <Tooltip contentStyle={TT_CONTENT} itemStyle={TT_ITEM} labelStyle={TT_LABEL}
                  cursor={TT_CURSOR}
                  formatter={(v, _, props) => [`${v}%  ·  ${props.payload.label}`, 'Memória']} />
                <Bar dataKey='pct' radius={[0, 3, 3, 0]} maxBarSize={14} background={{ fill: 'rgba(255,255,255,0.03)', radius: 3 }}>
                  {items.map((d, i) => (
                    <Cell key={i} fill={d.pct >= 90 ? '#f87171' : d.pct >= 75 ? '#fbbf24' : '#38bdf8'} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>
    )
  }

  if (type === 'cpu-line' || type === 'mem-line') {
    return <TimeSeriesWidget type={type} cluster={cluster} refreshTick={refreshTick} lang={lang} />
  }

  if (type === 'pods-not-running') {
    const statusPods = summary.pods?.status_pods || {}
    const rows = []
    Object.entries(statusPods).forEach(([status, names]) => {
      if (status === 'Running') return
      names.forEach(fullName => {
        const [ns, pod] = fullName.includes('/') ? fullName.split('/') : ['—', fullName]
        rows.push({ status, ns, pod })
      })
    })
    rows.sort((a, b) => a.status.localeCompare(b.status) || a.pod.localeCompare(b.pod))
    return (
      <div className='dw dw-table'>
        <PanelHeader type={type} title={t('widgetCatalog')['pods-not-running']} />
        <div className='dw-body' style={{ padding: '4px 0 0', overflow: 'auto' }}>
          {rows.length === 0 ? (
            <div className='dw-loading' style={{ color: '#34d399' }}>{t('allRunning')}</div>
          ) : (
            <table className='dw-tbl'>
              <thead><tr><th>{t('thPod')}</th><th>{t('thNamespace')}</th><th>{t('thStatus')}</th></tr></thead>
              <tbody>
                {rows.map((r, i) => (
                  <tr key={i}>
                    <td style={{ fontFamily: 'monospace', fontSize: 11 }}>{r.pod}</td>
                    <td className='dw-muted' style={{ fontSize: 11 }}>{r.ns}</td>
                    <td>
                      <span style={{
                        display: 'inline-block', padding: '1px 7px', borderRadius: 4,
                        fontSize: 10, fontWeight: 600,
                        background: podStatusColor(r.status) + '22',
                        color: podStatusColor(r.status),
                        border: `1px solid ${podStatusColor(r.status)}44`,
                      }}>{r.status}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    )
  }

  return <div className='dw-loading'>{t('unknownWidget')}</div>
}

function DashboardPage({ cluster, lang }) {
  const t = useT(lang || 'pt')
  const [dashboards,    setDashboards]    = useState([])
  const [activeDash,    setActiveDash]    = useState(null)
  const [editMode,      setEditMode]      = useState(false)
  const [summary,       setSummary]       = useState(null)
  const [summaryLoad,   setSummaryLoad]   = useState(false)
  const [refreshTick,   setRefreshTick]   = useState(0)
  const [showCatalog,   setShowCatalog]   = useState(false)
  const [saving,        setSaving]        = useState(false)
  const [newDashModal,  setNewDashModal]  = useState(false)
  const [dashName,      setDashName]      = useState('')
  const [gridWidth,     setGridWidth]     = useState(1200)
  const [rowHeight,     setRowHeight]     = useState(80)
  const gridRef = useRef(null)

  function uid() { return Math.random().toString(36).slice(2, 9) }

  useEffect(() => { loadDashboards() }, [])

  useEffect(() => {
    if (activeDash) loadSummary()
  }, [cluster, activeDash?.id])

  useEffect(() => {
    if (!activeDash) return
    // Tenta SSE primeiro, cai no polling de 5min como fallback
    const token = localStorage.getItem('pm_token')
    if (typeof EventSource !== 'undefined' && token) {
      const url = `/api/sse/events?cluster=${encodeURIComponent(cluster || '')}`
      const es = new EventSource(url, { withCredentials: false })
      // O token vai via interceptor do axios, mas SSE não suporta headers.
      // Fallback: polling continua como backup
      es.addEventListener('summary', e => {
        try {
          const data = JSON.parse(e.data)
          setSummary(data)
          setRefreshTick(prev => prev + 1)
        } catch {}
      })
      es.onerror = () => { es.close() }
      const timer = setInterval(loadSummary, 300_000)
      return () => { es.close(); clearInterval(timer) }
    }
    const timer = setInterval(loadSummary, 300_000) // fallback polling a cada 5min
    return () => clearInterval(timer)
  }, [cluster, activeDash?.id])

  useEffect(() => {
    function upd() {
      if (!gridRef.current) return
      setGridWidth(gridRef.current.offsetWidth)
      // calcula rowHeight para preencher a altura disponível com ~7 linhas
      const availH = gridRef.current.offsetHeight - 20 // padding
      setRowHeight(Math.max(60, Math.floor(availH / 7)))
    }
    upd()
    const ro = new ResizeObserver(upd)
    ro.observe(document.documentElement)
    return () => ro.disconnect()
  }, [])

  async function loadDashboards() {
    try {
      const { data } = await axios.get('/api/dashboards')
      const list = data || []
      setDashboards(list)
      if (list.length > 0) selectDash(list[0])
    } catch {}
  }

  function selectDash(d) {
    try { setActiveDash({ ...d, widgets: JSON.parse(d.widgets || '[]') }) }
    catch { setActiveDash({ ...d, widgets: [] }) }
  }

  async function loadSummary() {
    setSummaryLoad(true)
    try {
      const { data } = await axios.get('/api/dashboard/summary', { params: { cluster } })
      setSummary(data)
      setRefreshTick(t => t + 1)
    } catch {} finally { setSummaryLoad(false) }
  }

  async function createDashboard() {
    const name = dashName.trim() || t('newDashboard')
    const defaultWidgets = [
      { i: uid(), type: 'pods-summary',   x: 0, y: 0, w: 4, h: 3 },
      { i: uid(), type: 'nodes-summary',  x: 4, y: 0, w: 4, h: 3 },
      { i: uid(), type: 'alerts',         x: 8, y: 0, w: 4, h: 3 },
      { i: uid(), type: 'top-cpu',        x: 0, y: 3, w: 6, h: 4 },
      { i: uid(), type: 'top-mem',        x: 6, y: 3, w: 6, h: 4 },
    ]
    try {
      const { data } = await axios.post('/api/dashboards/save', { id: 0, name, widgets: JSON.stringify(defaultWidgets) })
      const newD = { id: data.id, name, username: '', widgets: JSON.stringify(defaultWidgets) }
      setDashboards(prev => [...prev, newD])
      setActiveDash({ ...newD, widgets: defaultWidgets })
      setNewDashModal(false); setDashName('')
    } catch {}
  }

  async function saveDashboard() {
    if (!activeDash) return
    setSaving(true)
    try {
      await axios.post('/api/dashboards/save', {
        id: activeDash.id, name: activeDash.name,
        widgets: JSON.stringify(activeDash.widgets)
      })
      setDashboards(prev => prev.map(d => d.id === activeDash.id ? { ...d, widgets: JSON.stringify(activeDash.widgets), name: activeDash.name } : d))
      setEditMode(false)
    } catch {} finally { setSaving(false) }
  }

  async function deleteDashboard(id) {
    if (!confirm(t('removeDashConfirm'))) return
    await axios.delete(`/api/dashboards/delete?id=${id}`)
    const rest = dashboards.filter(d => d.id !== id)
    setDashboards(rest)
    if (activeDash?.id === id) rest.length > 0 ? selectDash(rest[0]) : setActiveDash(null)
  }

  function addWidget(type, w, h) {
    const nw = { i: uid(), type, x: 0, y: Infinity, w, h }
    setActiveDash(prev => ({ ...prev, widgets: [...(prev.widgets || []), nw] }))
    setShowCatalog(false)
  }

  function removeWidget(i) {
    setActiveDash(prev => ({ ...prev, widgets: prev.widgets.filter(w => w.i !== i) }))
  }

  function onLayoutChange(layout) {
    if (!editMode || !activeDash) return
    setActiveDash(prev => ({
      ...prev,
      widgets: (prev.widgets || []).map(w => {
        const l = layout.find(l => l.i === w.i)
        return l ? { ...w, x: l.x, y: l.y, w: l.w, h: l.h } : w
      })
    }))
  }

  const layout = (activeDash?.widgets || []).map(w => ({ i: w.i, x: w.x, y: w.y, w: w.w, h: w.h, minW: 2, minH: 2 }))

  return (
    <div className='dash-page'>
      <div className='dash-header'>
        <div className='dash-tabs'>
          {dashboards.map(d => (
            <button key={d.id} className={`dash-tab ${activeDash?.id === d.id ? 'active' : ''}`}
              onClick={() => selectDash(d)}>
              {d.name}
              {editMode && activeDash?.id === d.id && (
                <span className='dash-tab-del' onClick={e => { e.stopPropagation(); deleteDashboard(d.id) }}>×</span>
              )}
            </button>
          ))}
          <button className='dash-tab-new' onClick={() => setNewDashModal(true)} title='Novo dashboard'>＋</button>
        </div>
        <div className='dash-actions'>
          <button className='dash-btn' onClick={loadSummary} disabled={summaryLoad} title={t('refreshBtn')}>
            <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2.5' style={{width:13,height:13}}>
              <polyline points='23 4 23 10 17 10'/><path d='M20.49 15a9 9 0 1 1-2.12-9.36L23 10'/>
            </svg>
          </button>
          {activeDash && !editMode && <button className='dash-btn' onClick={() => setEditMode(true)}>{t('edit')}</button>}
          {editMode && <>
            <button className='dash-btn' onClick={() => setShowCatalog(true)}>{t('addWidgetBtn')}</button>
            <button className='dash-btn dash-btn-save' onClick={saveDashboard} disabled={saving}>{saving ? t('savingDash') : t('saveDash')}</button>
            <button className='dash-btn' onClick={() => setEditMode(false)}>{t('cancelDash')}</button>
          </>}
        </div>
      </div>

      {newDashModal && (
        <div className='modal-overlay' onClick={() => setNewDashModal(false)}>
          <div className='modal-box' onClick={e => e.stopPropagation()} style={{maxWidth:340}}>
            <div className='modal-header'><span>{t('newDashboard')}</span><button className='modal-close' onClick={() => setNewDashModal(false)}>✕</button></div>
            <div className='modal-body' style={{padding:'1rem'}}>
              <input className='dash-name-input' placeholder={t('dashNamePlaceholder')} autoFocus
                value={dashName} onChange={e => setDashName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && createDashboard()} />
            </div>
            <div className='modal-footer'>
              <button onClick={() => setNewDashModal(false)}>{t('cancel')}</button>
              <button className='modal-save' onClick={createDashboard}>{t('createDashBtn')}</button>
            </div>
          </div>
        </div>
      )}

      {showCatalog && (
        <div className='modal-overlay' onClick={() => setShowCatalog(false)}>
          <div className='modal-box' onClick={e => e.stopPropagation()} style={{maxWidth:480}}>
            <div className='modal-header'><span>{t('addWidget')}</span><button className='modal-close' onClick={() => setShowCatalog(false)}>✕</button></div>
            <div className='dash-catalog'>
              {WIDGET_CATALOG_TYPES.map(w => (
                <button key={w.type} className='dash-catalog-item' onClick={() => addWidget(w.type, w.w, w.h)}>
                  <span className='dash-catalog-icon'>{WIDGET_ICONS[w.type]}</span>
                  <span className='dash-catalog-label'>{t('widgetCatalog')[w.type]}</span>
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      {!activeDash ? (
        <div className='dash-empty'>
          <p>{t('noDashboardYet')}</p>
          <button className='dash-btn dash-btn-save' onClick={() => setNewDashModal(true)}>{t('createDashboard')}</button>
        </div>
      ) : (
        <div className='dash-grid-wrap' ref={gridRef}>
          {summaryLoad && <div className='dash-loading'>{t('updatingData')}</div>}
          <GridLayout
            className='dash-grid'
            layout={layout}
            cols={12}
            rowHeight={rowHeight}
            width={gridWidth || 1200}
            isDraggable={editMode}
            isResizable={editMode}
            onLayoutChange={onLayoutChange}
            margin={[12, 12]}
            containerPadding={[0, 0]}
            draggableHandle='.dw-title'>
            {(activeDash.widgets || []).map(w => (
              <div key={w.i} className={`dash-widget ${editMode ? 'editing' : ''}`}>
                {editMode && <button className='dash-widget-remove' onClick={() => removeWidget(w.i)}>×</button>}
                <DashWidget type={w.type} summary={summary} cluster={cluster} refreshTick={refreshTick} lang={lang} />
              </div>
            ))}
          </GridLayout>
        </div>
      )}
    </div>
  )
}

// ── Topology Graph ────────────────────────────────────────────────────────────
const NODE_COLORS = {
  Pod:         '#4a9eff',
  Service:     '#f59e0b',
  Deployment:  '#10b981',
  ConfigMap:   '#a78bfa',
  Secret:      '#f87171',
  HPA:         '#fb923c',
  StatefulSet: '#06b6d4',
  DaemonSet:   '#84cc16',
  ReplicaSet:  '#64748b',
  Job:         '#f472b6',
  CronJob:     '#c084fc',
  Ingress:     '#22d3ee',
}
const NODE_RADII = {
  Pod: 22, Service: 26, Deployment: 26, ConfigMap: 20, Secret: 20,
  HPA: 22, StatefulSet: 26, DaemonSet: 26, ReplicaSet: 20, Job: 22, CronJob: 24, Ingress: 24,
}
const EDGE_COLORS = {
  owns: '#10b981', selects: '#f59e0b', mounts: '#a78bfa',
  env: '#94a3b8', scales: '#fb923c', routes: '#22d3ee', spawns: '#c084fc',
  connects: '#ef4444',
}

function escapeXml(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;')
}

function downloadTopologyAsDrawio(nodes, edges, positions, t) {
  const idMap = new Map()
  nodes.forEach((n, i) => idMap.set(n.id, `n${i}`))
  const edgeLabel = type => {
    const key = 'topoEdge' + type.charAt(0).toUpperCase() + type.slice(1)
    return (t && t(key)) || type
  }

  const xs = nodes.map(n => positions[n.id]?.x ?? 0)
  const ys = nodes.map(n => positions[n.id]?.y ?? 0)
  const offX = -Math.min(0, ...xs) + 60
  const offY = -Math.min(0, ...ys) + 60

  // Nome fica FORA do símbolo (acima), com largura fixa para quebrar em
  // várias linhas — evita que nomes longos estourem o círculo colorido,
  // que serve só como indicador visual do tipo (cor = kind, ver legenda).
  const LABEL_WIDTH = 130
  let maxBottom = 0
  const vertices = nodes.map(n => {
    const pos = positions[n.id]
    if (!pos) return ''
    const r = NODE_RADII[n.kind] || 20
    const size = r * 2
    const color = NODE_COLORS[n.kind] || '#94a3b8'
    const x = pos.x + offX - r
    const y = pos.y + offY - r
    maxBottom = Math.max(maxBottom, y + size)
    const label = `&lt;div style='width:${LABEL_WIDTH}px;'&gt;&lt;b style='color:${color};'&gt;${escapeXml(n.kind)}&lt;/b&gt;&lt;br&gt;${escapeXml(n.name)}&lt;/div&gt;`
    return `<mxCell id="${idMap.get(n.id)}" value="${label}" style="ellipse;whiteSpace=wrap;html=1;fillColor=${color};fillOpacity=25;strokeColor=${color};strokeWidth=2;fontSize=10;verticalLabelPosition=top;verticalAlign=bottom;align=center;" vertex="1" parent="1"><mxGeometry x="${x.toFixed(1)}" y="${y.toFixed(1)}" width="${size}" height="${size}" as="geometry" /></mxCell>`
  }).join('')

  const edgeCells = edges
    .filter(e => idMap.has(e.from) && idMap.has(e.to))
    .map((e, i) => {
      const color = EDGE_COLORS[e.type] || '#64748b'
      const dashed = (e.type === 'env' || e.type === 'spawns' || e.type === 'connects') ? 'dashed=1;dashPattern=4 3;' : ''
      return `<mxCell id="e${i}" value="${escapeXml(edgeLabel(e.type))}" style="endArrow=classic;html=1;strokeColor=${color};strokeWidth=1.5;fontSize=8;${dashed}" edge="1" parent="1" source="${idMap.get(e.from)}" target="${idMap.get(e.to)}"><mxGeometry relative="1" as="geometry" /></mxCell>`
    }).join('')

  // Legenda (cores dos nós + tipos de aresta), posicionada abaixo do grafo.
  const legendX = 60
  let legendY = maxBottom + 60
  const legendCells = []
  let legendSeq = 0
  const nextLegendId = () => `legend${legendSeq++}`
  const COLS = 4
  const ITEM_W = 190
  const ITEM_H = 26

  legendCells.push(`<mxCell id="${nextLegendId()}" value="${escapeXml(t ? t('topoTitle') : 'Legenda')}" style="text;html=1;fontSize=14;fontStyle=1;align=left;" vertex="1" parent="1"><mxGeometry x="${legendX}" y="${legendY}" width="300" height="24" as="geometry" /></mxCell>`)
  legendY += 34

  const nodeEntries = Object.entries(NODE_COLORS)
  nodeEntries.forEach(([kind, color], i) => {
    const col = i % COLS
    const row = Math.floor(i / COLS)
    const x = legendX + col * ITEM_W
    const y = legendY + row * ITEM_H
    legendCells.push(`<mxCell id="${nextLegendId()}" style="ellipse;html=1;fillColor=${color};fillOpacity=30;strokeColor=${color};strokeWidth=1.5;" vertex="1" parent="1"><mxGeometry x="${x}" y="${y + 4}" width="16" height="16" as="geometry" /></mxCell>`)
    legendCells.push(`<mxCell id="${nextLegendId()}" value="${escapeXml(kind)}" style="text;html=1;fontSize=11;align=left;verticalAlign=middle;" vertex="1" parent="1"><mxGeometry x="${x + 22}" y="${y}" width="${ITEM_W - 24}" height="24" as="geometry" /></mxCell>`)
  })
  legendY += Math.ceil(nodeEntries.length / COLS) * ITEM_H + 24

  legendCells.push(`<mxCell id="${nextLegendId()}" value="${escapeXml(t ? t('topoConnections') : 'Conexões')}" style="text;html=1;fontSize=14;fontStyle=1;align=left;" vertex="1" parent="1"><mxGeometry x="${legendX}" y="${legendY}" width="300" height="24" as="geometry" /></mxCell>`)
  legendY += 34

  const edgeEntries = Object.entries(EDGE_COLORS)
  edgeEntries.forEach(([type, color], i) => {
    const col = i % COLS
    const row = Math.floor(i / COLS)
    const x = legendX + col * ITEM_W
    const y = legendY + row * ITEM_H
    const dashed = (type === 'env' || type === 'spawns' || type === 'connects') ? 'dashed=1;dashPattern=4 3;' : ''
    legendCells.push(`<mxCell id="${nextLegendId()}" value="" style="endArrow=none;html=1;strokeColor=${color};strokeWidth=2;${dashed}" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="${x}" y="${y + 10}" as="sourcePoint" /><mxPoint x="${x + 28}" y="${y + 10}" as="targetPoint" /></mxGeometry></mxCell>`)
    legendCells.push(`<mxCell id="${nextLegendId()}" value="${escapeXml(edgeLabel(type))}" style="text;html=1;fontSize=11;align=left;verticalAlign=middle;" vertex="1" parent="1"><mxGeometry x="${x + 34}" y="${y}" width="${ITEM_W - 36}" height="24" as="geometry" /></mxCell>`)
  })

  const xml = `<?xml version="1.0" encoding="UTF-8"?>\n<mxfile host="pod-monitor"><diagram name="Topology" id="topology"><mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="1600" pageHeight="1200" math="0" shadow="0"><root><mxCell id="0" /><mxCell id="1" parent="0" />${vertices}${edgeCells}${legendCells.join('')}</root></mxGraphModel></diagram></mxfile>`

  const blob = new Blob([xml], { type: 'application/xml' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  const ts = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
  a.download = `topology-${ts}.drawio`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function TopoGraph({ nodes, edges, lang }) {
  const t = useT(lang || 'pt')
  const svgRef = useRef(null)
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 })
  const [dragging, setDragging] = useState(null)
  const [positions, setPositions] = useState({})
  const [selected, setSelected] = useState(null)
  const [layoutMode, setLayoutMode] = useState('force')
  const [topoSearch, setTopoSearch] = useState('')
  const [hiddenKinds, setHiddenKinds] = useState(new Set())
  const [hideEmptyRS, setHideEmptyRS] = useState(true)

  // Nós e arestas visíveis (filtros de tipo + RS vazio)
  const visibleNodes = nodes.filter(n => {
    if (hiddenKinds.has(n.kind)) return false
    if (hideEmptyRS && n.kind === 'ReplicaSet' && n.meta?.availableReplicas === '0') return false
    return true
  })
  const visibleNodeIds = new Set(visibleNodes.map(n => n.id))
  const visibleEdges = edges.filter(e => visibleNodeIds.has(e.from) && visibleNodeIds.has(e.to))
  const visibleNodeKey = visibleNodes.map(n => n.id).sort().join(',')

  function matchesSearch(n) {
    if (!topoSearch) return true
    try { return new RegExp(topoSearch, 'i').test(n.name) } catch { return n.name.toLowerCase().includes(topoSearch.toLowerCase()) }
  }

  function toggleKind(kind) {
    setHiddenKinds(prev => {
      const next = new Set(prev)
      if (next.has(kind)) next.delete(kind)
      else next.add(kind)
      return next
    })
  }

  function centerGraph() {
    const el = svgRef.current
    if (!el) return
    const { width, height } = el.getBoundingClientRect()
    setTransform({ x: width / 2, y: height / 2, scale: 1 })
  }

  useEffect(() => {
    if (visibleNodes.length === 0) return
    if (layoutMode === 'circle') {
      const byKind = {}
      visibleNodes.forEach(n => { if (!byKind[n.kind]) byKind[n.kind] = []; byKind[n.kind].push(n) })
      const kinds = Object.keys(byKind)
      const pos = {}
      const ringR = [0, 120, 220, 310, 400, 490, 580]
      kinds.forEach((kind, ki) => {
        const r = ringR[ki] || (ki * 90 + 100)
        byKind[kind].forEach((n, i) => {
          const angle = (2 * Math.PI * i / byKind[kind].length) - Math.PI / 2
          pos[n.id] = { x: r * Math.cos(angle), y: r * Math.sin(angle) }
        })
      })
      setPositions(pos)
    } else {
      const simNodes = visibleNodes.map(n => ({ id: n.id, r: NODE_RADII[n.kind] || 20 }))
      const nodeById = Object.fromEntries(simNodes.map(n => [n.id, n]))
      const simLinks = visibleEdges
        .filter(e => nodeById[e.from] && nodeById[e.to])
        .map(e => ({ source: e.from, target: e.to }))
      forceSimulation(simNodes)
        .force('link', forceLink(simLinks).id(d => d.id).distance(110).strength(0.4))
        .force('charge', forceManyBody().strength(-350))
        .force('center', forceCenter(0, 0))
        .force('collide', forceCollide(d => d.r + 18))
        .stop()
        .tick(300)
      const pos = {}
      simNodes.forEach(n => { pos[n.id] = { x: n.x || 0, y: n.y || 0 } })
      setPositions(pos)
    }
    requestAnimationFrame(centerGraph)
  }, [visibleNodeKey, layoutMode])

  function onNodeMouseDown(e, id) {
    e.stopPropagation()
    setSelected(id)
    setDragging({ type: 'node', id })
  }

  function onSvgMouseDown(e) {
    if (e.button !== 0) return
    setDragging({ type: 'pan', startX: e.clientX, startY: e.clientY, startTx: transform.x, startTy: transform.y })
  }

  function onMouseMove(e) {
    if (!dragging) return
    if (dragging.type === 'pan') {
      setTransform(prev => ({ ...prev, x: dragging.startTx + e.clientX - dragging.startX, y: dragging.startTy + e.clientY - dragging.startY }))
    } else if (dragging.type === 'node') {
      const svgRect = svgRef.current?.getBoundingClientRect()
      if (!svgRect) return
      const x = (e.clientX - svgRect.left - transform.x) / transform.scale
      const y = (e.clientY - svgRect.top  - transform.y) / transform.scale
      setPositions(prev => ({ ...prev, [dragging.id]: { x, y } }))
    }
  }

  function onMouseUp() { setDragging(null) }

  function onWheel(e) {
    e.preventDefault()
    const factor = e.deltaY < 0 ? 1.15 : 0.87
    const svgRect = svgRef.current?.getBoundingClientRect()
    if (!svgRect) return
    const mx = e.clientX - svgRect.left
    const my = e.clientY - svgRect.top
    setTransform(prev => {
      const newScale = Math.max(0.2, Math.min(3, prev.scale * factor))
      const ratio = newScale / prev.scale
      return { x: mx - (mx - prev.x) * ratio, y: my - (my - prev.y) * ratio, scale: newScale }
    })
  }

  const selNode = selected ? nodes.find(n => n.id === selected) : null
  const isPanning = dragging?.type === 'pan'
  const hasSearch = topoSearch.length > 0

  return (
    <div style={{ position: 'relative', width: '100%', height: 'calc(100vh - 190px)', background: 'var(--bg-2)', borderRadius: 8, overflow: 'hidden', border: '1px solid var(--border)' }}>

      {/* Barra de busca */}
      <div style={{ position: 'absolute', top: 8, left: '50%', transform: 'translateX(-50%)', zIndex: 10, display: 'flex', gap: 6, alignItems: 'center' }}>
        <input
          type='text'
          value={topoSearch}
          onChange={e => setTopoSearch(e.target.value)}
          placeholder={t('topoSearchPlaceholder')}
          style={{ width: 200, fontSize: 12, padding: '3px 8px', borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg-1)', color: 'var(--text-1)' }}
        />
        {hasSearch && (
          <span style={{ fontSize: 11, color: 'var(--text-3)' }}>
            {visibleNodes.filter(matchesSearch).length}/{visibleNodes.length}
          </span>
        )}
      </div>

      <svg ref={svgRef} width='100%' height='100%'
        onMouseDown={onSvgMouseDown}
        onMouseMove={onMouseMove} onMouseUp={onMouseUp} onMouseLeave={onMouseUp}
        onWheel={onWheel} style={{ cursor: isPanning ? 'grabbing' : 'grab', userSelect: 'none' }}>
        <defs>
          <marker id='arrowhead' markerWidth='10' markerHeight='7' refX='10' refY='3.5' orient='auto'>
            <polygon points='0 0, 10 3.5, 0 7' fill='#64748b' />
          </marker>
        </defs>
        <g transform={`translate(${transform.x},${transform.y}) scale(${transform.scale})`}>
          {/* Edges */}
          {visibleEdges.map((e, i) => {
            const from = positions[e.from], to = positions[e.to]
            if (!from || !to) return null
            const color = EDGE_COLORS[e.type] || '#64748b'
            const dx = to.x - from.x, dy = to.y - from.y
            const len = Math.sqrt(dx * dx + dy * dy) || 1
            const r = NODE_RADII[visibleNodes.find(n => n.id === e.to)?.kind] || 20
            const ex = to.x - dx / len * (r + 3), ey = to.y - dy / len * (r + 3)
            const isDashed = e.type === 'env' || e.type === 'spawns' || e.type === 'connects'
            return (
              <line key={i} x1={from.x} y1={from.y} x2={ex} y2={ey}
                stroke={color} strokeWidth={1.5} strokeOpacity={0.7}
                markerEnd='url(#arrowhead)' strokeDasharray={isDashed ? '4,3' : undefined} />
            )
          })}
          {/* Nodes */}
          {visibleNodes.map(n => {
            const pos = positions[n.id]
            if (!pos) return null
            const r = NODE_RADII[n.kind] || 20
            const color = NODE_COLORS[n.kind] || '#94a3b8'
            const statusStroke = n.status === 'error' ? '#ef4444' : n.status === 'warn' ? '#f59e0b' : color
            const isSelected = selected === n.id
            const dimmed = hasSearch && !matchesSearch(n)
            const shortName = n.name.length > 16 ? n.name.slice(0, 14) + '…' : n.name
            return (
              <g key={n.id} transform={`translate(${pos.x},${pos.y})`}
                onMouseDown={e => onNodeMouseDown(e, n.id)}
                style={{ cursor: 'pointer', opacity: dimmed ? 0.12 : 1, transition: 'opacity 0.15s' }}>
                <circle r={r} fill={color} fillOpacity={0.2}
                  stroke={statusStroke} strokeWidth={isSelected ? 3 : 2}
                  filter={isSelected ? 'drop-shadow(0 0 6px rgba(255,255,255,0.4))' : undefined} />
                <text fontSize={9} fill='var(--text-1)' textAnchor='middle' dy={-r - 4}
                  style={{ pointerEvents: 'none', fontWeight: 600 }}>{shortName}</text>
                <text fontSize={7} fill={color} textAnchor='middle' dy={3}
                  style={{ pointerEvents: 'none' }}>{n.kind}</text>
              </g>
            )
          })}
        </g>
      </svg>

      {/* Controles de zoom e layout */}
      <div style={{ position: 'absolute', top: 8, right: 8, display: 'flex', gap: 4, flexDirection: 'column' }}>
        <button onClick={() => setTransform(p => ({ ...p, scale: Math.min(3, p.scale * 1.2) }))} title={t('topoZoomIn')}
          style={{ width: 28, height: 28, borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg-1)', color: 'var(--text-1)', cursor: 'pointer', fontSize: 14 }}>+</button>
        <button onClick={() => setTransform(p => ({ ...p, scale: Math.max(0.2, p.scale / 1.2) }))} title={t('topoZoomOut')}
          style={{ width: 28, height: 28, borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg-1)', color: 'var(--text-1)', cursor: 'pointer', fontSize: 14 }}>−</button>
        <button onClick={centerGraph} title={t('topoReset')}
          style={{ width: 28, height: 28, borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg-1)', color: 'var(--text-1)', cursor: 'pointer', fontSize: 14 }}>⊙</button>
        <button
          onClick={() => setLayoutMode(m => m === 'force' ? 'circle' : 'force')}
          title={layoutMode === 'force' ? t('topoSwitchToCircle') : t('topoSwitchToForce')}
          style={{ width: 28, height: 28, borderRadius: 4, border: '1px solid var(--border)', background: layoutMode === 'force' ? 'var(--accent)' : 'var(--bg-1)', color: layoutMode === 'force' ? '#fff' : 'var(--text-1)', cursor: 'pointer', fontSize: 11, fontWeight: 700 }}>
          {layoutMode === 'force' ? '⬡' : '◎'}
        </button>
        <button
          onClick={() => downloadTopologyAsDrawio(visibleNodes, visibleEdges, positions, t)}
          title={t('topoDownloadDrawio')}
          disabled={visibleNodes.length === 0}
          style={{ width: 28, height: 28, borderRadius: 4, border: '1px solid var(--border)', background: 'var(--bg-1)', color: 'var(--text-1)', cursor: visibleNodes.length === 0 ? 'default' : 'pointer', fontSize: 14 }}>
          ⬇
        </button>
      </div>

      {/* Legenda interativa */}
      <div style={{ position: 'absolute', bottom: 8, left: 8, display: 'flex', flexDirection: 'column', gap: 6, background: 'var(--bg-1)', border: '1px solid var(--border)', borderRadius: 6, padding: '6px 10px', maxWidth: 320 }}>
        {/* Toggle RS vazio */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 10, color: 'var(--text-3)', borderBottom: '1px solid var(--border)', paddingBottom: 4, marginBottom: 2 }}>
          <input type='checkbox' id='hideEmptyRS' checked={hideEmptyRS} onChange={e => setHideEmptyRS(e.target.checked)} style={{ cursor: 'pointer' }} />
          <label htmlFor='hideEmptyRS' style={{ cursor: 'pointer' }}>{t('topoHideEmptyRS')}</label>
        </div>
        {/* Nós — clicáveis para filtrar */}
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {Object.entries(NODE_COLORS).map(([kind, color]) => {
            const hidden = hiddenKinds.has(kind)
            return (
              <span key={kind} onClick={() => toggleKind(kind)} title={hidden ? t('topoShowKind') : t('topoHideKind')}
                style={{ fontSize: 10, color: hidden ? 'var(--text-3)' : 'var(--text-2)', display: 'flex', alignItems: 'center', gap: 3, cursor: 'pointer', opacity: hidden ? 0.4 : 1, userSelect: 'none' }}>
                <svg width={12} height={12}><circle cx={6} cy={6} r={5} fill={color} fillOpacity={hidden ? 0.1 : 0.3} stroke={color} strokeWidth={1.5} /></svg>
                {kind}
              </span>
            )
          })}
        </div>
        {/* Arestas */}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {Object.entries(EDGE_COLORS).map(([type, color]) => {
            const isDashed = type === 'env' || type === 'spawns' || type === 'connects'
            return (
              <span key={type} style={{ fontSize: 10, color: 'var(--text-2)', display: 'flex', alignItems: 'center', gap: 3 }}>
                <svg width={20} height={10}><line x1={0} y1={5} x2={20} y2={5} stroke={color} strokeWidth={1.5} strokeDasharray={isDashed ? '3,2' : undefined} /></svg>
                {t('topoEdge' + type.charAt(0).toUpperCase() + type.slice(1)) || type}
              </span>
            )
          })}
        </div>
      </div>

      {/* Painel de detalhes */}
      {selNode && (
        <div style={{ position: 'absolute', top: 44, left: 8, background: 'var(--bg-1)', border: '1px solid var(--border)', borderRadius: 6, padding: '8px 12px', minWidth: 180, maxWidth: 260, fontSize: 12 }}>
          <div style={{ fontWeight: 700, color: NODE_COLORS[selNode.kind] || 'var(--text-1)', marginBottom: 4 }}>{selNode.kind}</div>
          <div style={{ color: 'var(--text-1)', wordBreak: 'break-all' }}>{selNode.name}</div>
          <div style={{ color: 'var(--text-3)', fontSize: 11, marginTop: 2 }}>{selNode.namespace}</div>
          {selNode.kind === 'HPA' && selNode.meta && (
            <div style={{ marginTop: 6, fontSize: 11, color: 'var(--text-2)', display: 'flex', flexDirection: 'column', gap: 2 }}>
              <span>{t('hpaTarget')}: <b>{selNode.meta.targetKind}/{selNode.meta.targetName}</b></span>
              <span>{t('hpaReplicas')}: <b style={{ color: '#4ade80' }}>{selNode.meta.currentReplicas}</b> / {selNode.meta.desiredReplicas} ({t('hpaMin')} {selNode.meta.minReplicas}, {t('hpaMax')} {selNode.meta.maxReplicas})</span>
            </div>
          )}
          {selNode.kind === 'ReplicaSet' && selNode.meta && (
            <div style={{ marginTop: 6, fontSize: 11, color: 'var(--text-2)' }}>
              {t('topoRSReplicas')}: <b style={{ color: '#4ade80' }}>{selNode.meta.availableReplicas}</b> / {selNode.meta.replicas}
            </div>
          )}
          <div style={{ marginTop: 6, fontSize: 10, color: 'var(--text-3)' }}>
            {t('topoNodes')(edges.filter(e => e.from === selNode.id || e.to === selNode.id).length)} {t('topoConnections')}
          </div>
          <button onClick={() => setSelected(null)} style={{ position: 'absolute', top: 4, right: 4, background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-3)', fontSize: 14 }}>×</button>
        </div>
      )}
    </div>
  )
}

// ── App principal ─────────────────────────────────────────────────────────────
export default function App() {
  // Auth
  const [user,      setUser]      = useState(() => {
    try { return JSON.parse(localStorage.getItem('pm_user')) } catch { return null }
  })
  const [currentView,  setCurrentView]  = useState('main') // 'main' | 'users'
  const [gearOpen,     setGearOpen]     = useState(false)
  const [helpOpen,     setHelpOpen]     = useState(false)
  const gearRef  = useRef(null)
  const themeRef = useRef(null)
  const [theme,     setTheme]     = useState(() => localStorage.getItem('pm_theme') || 'dark')
  const [themeOpen, setThemeOpen] = useState(false)
  const [navLayout, setNavLayout] = useState(() => localStorage.getItem('pm_nav_layout') || 'top')
  useEffect(() => { localStorage.setItem('pm_nav_layout', navLayout) }, [navLayout])

  // Language
  const [lang, setLang] = useState(() => localStorage.getItem('pm_lang') || 'pt')
  useEffect(() => { localStorage.setItem('pm_lang', lang) }, [lang])
  const t = useT(lang)

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  function applyTheme(id) {
    setTheme(id)
    localStorage.setItem('pm_theme', id)
    setThemeOpen(false)
  }

  useEffect(() => {
    function handleClickOutside(e) {
      if (gearRef.current  && !gearRef.current.contains(e.target))  setGearOpen(false)
      if (themeRef.current && !themeRef.current.contains(e.target)) setThemeOpen(false)
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // Configura axios com token
  useEffect(() => {
    const interceptor = axios.interceptors.request.use(cfg => {
      const stored = localStorage.getItem('pm_token')
      if (stored) cfg.headers.Authorization = `Bearer ${stored}`
      return cfg
    })
    const respInterceptor = axios.interceptors.response.use(
      r => r,
      err => {
        if (err.response?.status === 401) logout()
        return Promise.reject(err)
      }
    )
    return () => {
      axios.interceptors.request.eject(interceptor)
      axios.interceptors.response.eject(respInterceptor)
    }
  }, [])

  function handleLogin(data) {
    localStorage.setItem('pm_token', data.token)
    const u = {
      username:           data.username,
      role:               data.role,
      allowed_clusters:   data.allowed_clusters  || [],
      allowed_namespaces: data.allowed_namespaces || [],
    }
    localStorage.setItem('pm_user', JSON.stringify(u))
    setUser(u)
  }

  function logout() {
    localStorage.removeItem('pm_token')
    localStorage.removeItem('pm_user')
    setUser(null)
  }

  const isAdmin      = user?.role === 'administration'
  const isDev        = user?.role === 'dev'
  const isDevOrAdmin = user?.role === 'administration' || user?.role === 'dev'

  const tabLabels = t('tabs') || {}
  const allTabs = [
    { id: 'monitor',    label: tabLabels.monitor     || 'Monitor',       roles: 'adminOrReader' },
    { id: 'top10',      label: tabLabels.top10       || 'Top 10',        roles: 'adminOrReader' },
    { id: 'historico',  label: tabLabels.historico   || 'Histórico',     roles: 'adminOrReader' },
    { id: 'namespaces', label: tabLabels.namespaces  || 'Namespaces',    roles: 'adminOrReader' },
    { id: 'storage',    label: tabLabels.storage     || 'Storage',       roles: 'adminOrReader' },
    { id: 'orphans',    label: tabLabels.orphans     || 'Órfãos',        roles: 'adminOrReader' },
    { id: 'containers', label: tabLabels.containers  || 'Containers',    roles: 'adminOrReader' },
    { id: 'docker',     label: tabLabels.docker      || 'Docker/Podman', roles: 'adminOrReader' },
    { id: 'helm',       label: tabLabels.helm        || 'Helm',          roles: 'adminOrReader' },
    { id: 'deployments', label: tabLabels.deployments || 'Deployments',  roles: 'adminOrReader' },
    { id: 'dashboards',  label: tabLabels.dashboards  || 'Dashboards',   roles: 'adminOrReader' },
    { id: 'analysis',   label: tabLabels.analysis    || 'Análise',       roles: 'adminOrReader' },
    { id: 'topology',   label: tabLabels.topology    || 'Topologia',     roles: 'adminOrReader' },
    { id: 'quotas',     label: tabLabels.quotas      || 'Quotas',        roles: 'adminOrReader' },
    { id: 'logs',       label: tabLabels.logs        || 'Logs',          roles: 'devOrAdmin'    },
    { id: 'nodes',      label: tabLabels.nodes       || 'Nodes',         roles: 'admin'         },
    { id: 'admin',      label: tabLabels.admin       || 'Admin',         roles: 'admin'         },
  ]
  const visibleTabs = allTabs.filter(t => {
    if (t.roles === 'admin')        return isAdmin
    if (t.roles === 'devOrAdmin')   return isDevOrAdmin
    if (t.roles === 'adminOrReader') return !isDev
    return true
  })

  const [activeTab, setActiveTab] = useState(() =>
    (() => { try { const u = JSON.parse(localStorage.getItem('pm_user')); return u?.role === 'dev' ? 'logs' : 'monitor' } catch { return 'monitor' } })()
  )

  // Garante que a aba ativa é visível para o perfil
  useEffect(() => {
    if (!visibleTabs.find(t => t.id === activeTab)) {
      setActiveTab(isDev ? 'logs' : 'monitor')
    }
  }, [user])

  // clusters
  const [clusters,   setClusters]   = useState([])
  const [cluster,    setCluster]    = useState('')

  // NOC mode
  const [nocMode,     setNocMode]     = useState(() => { try { return JSON.parse(localStorage.getItem('pm_noc') || 'false') } catch { return false } })
  const [nocInterval, setNocInterval] = useState(() => parseInt(localStorage.getItem('pm_noc_interval') || '5', 10))
  const [nocModules,  setNocModules]  = useState(() => { try { return JSON.parse(localStorage.getItem('pm_noc_modules') || '["monitor","namespaces","containers"]') } catch { return ['monitor', 'namespaces', 'containers'] } })

  // monitor
  const [namespaces, setNamespaces] = useState([])
  const [namespace,  setNamespace]  = useState('')
  const [pods,       setPods]       = useState([])
  const [filter,     setFilter]     = useState('all')
  const [loading,    setLoading]    = useState(false)
  const [error,      setError]      = useState('')

  // histórico
  const [history,    setHistory]    = useState([])
  const [histNs,     setHistNs]     = useState('')
  const [histStart,    setHistStart]    = useState('')
  const [histEnd,      setHistEnd]      = useState('')
  const [activePreset, setActivePreset] = useState(null)
  const [histLoading,  setHistLoading]  = useState(false)
  const [histError,    setHistError]    = useState('')

  // nodes
  const [nodes,      setNodes]      = useState([])
  const [nodesLoad,  setNodesLoad]  = useState(false)
  const [nodesError, setNodesError] = useState('')

  // admin docker hosts
  const [dockerHostName,    setDockerHostName]    = useState('')
  const [dockerHostAddr,    setDockerHostAddr]    = useState('tcp://')
  const [dockerHostLoading, setDockerHostLoading] = useState(false)
  const [dockerHostMsg,     setDockerHostMsg]     = useState({ type: '', text: '' })

  async function addDockerHost() {
    setDockerHostLoading(true); setDockerHostMsg({ type: '', text: '' })
    try {
      await axios.post('/api/admin/docker-host', { name: dockerHostName.trim(), address: dockerHostAddr.trim() })
      setDockerHostMsg({ type: 'success', text: t('hostAdded')(dockerHostName) })
      setDockerHostName(''); setDockerHostAddr('tcp://')
      axios.get('/api/docker/hosts').then(({ data }) => setDockerHosts(data || []))
    } catch (err) {
      setDockerHostMsg({ type: 'error', text: err.response?.data || t('errorAddingHost') })
    } finally { setDockerHostLoading(false) }
  }

  async function removeDockerHost(name) {
    setDockerHostLoading(true); setDockerHostMsg({ type: '', text: '' })
    try {
      await axios.delete(`/api/admin/docker-host?name=${encodeURIComponent(name)}`)
      setDockerHostMsg({ type: 'success', text: t('hostRemoved')(name) })
      axios.get('/api/docker/hosts').then(({ data }) => setDockerHosts(data || []))
    } catch (err) {
      setDockerHostMsg({ type: 'error', text: err.response?.data || t('errorRemovingHost') })
    } finally { setDockerHostLoading(false) }
  }

  // admin cluster
  const [adminKubeconfig,   setAdminKubeconfig]   = useState('')
  const [adminContexts,     setAdminContexts]     = useState([])
  const [adminSelCtx,       setAdminSelCtx]       = useState('')
  const [adminNs,           setAdminNs]           = useState('default')
  const [adminSAKubeconfig, setAdminSAKubeconfig] = useState('')
  const [adminStep,         setAdminStep]         = useState(1)
  const [adminLoading,      setAdminLoading]      = useState(false)
  const [adminMsg,          setAdminMsg]          = useState({ type: '', text: '' })
  const [delClusterModal,   setDelClusterModal]   = useState(null)  // nome do cluster a deletar
  const [delClusterPwd,     setDelClusterPwd]     = useState('')
  const [delClusterErr,     setDelClusterErr]     = useState('')
  const [delClusterLoading, setDelClusterLoading] = useState(false)

  useEffect(() => {
    if (!user) return
    axios.get('/api/clusters')
      .then(r => { const list = r.data || []; setClusters(list); if (list.length > 0) setCluster(list[0]) })
      .catch(() => {})
  }, [user])

  useEffect(() => {
    if (!cluster) return
    setPods([]); setNodes([]); setHistory([]); setNamespace(''); setHistNs('')
    axios.get(`/api/namespaces?cluster=${cluster}`)
      .then(r => setNamespaces(r.data || []))
      .catch(() => setNamespaces([]))
    // auto-carregar todos os pods ao trocar de cluster
    setLoading(true); setError('')
    axios.get(`/api/resources?cluster=${cluster}`)
      .then(({ data }) => setPods(data || []))
      .catch(() => setError(t('errorApiConsult')))
      .finally(() => setLoading(false))
  }, [cluster])

  const cp = () => cluster ? `cluster=${cluster}` : ''

  // NOC: módulos ciclam a cada 1 min; cluster cicla a cada N min
  useEffect(() => {
    if (!nocMode || clusters.length === 0 || nocModules.length === 0) return
    let modIdx = Math.max(0, nocModules.indexOf(activeTab))
    let clsIdx = Math.max(0, clusters.indexOf(cluster))
    setActiveTab(nocModules[modIdx])
    const modTimer = setInterval(() => {
      modIdx = (modIdx + 1) % nocModules.length
      setActiveTab(nocModules[modIdx])
    }, 60_000)
    const clsTimer = setInterval(() => {
      clsIdx = (clsIdx + 1) % clusters.length
      setCluster(clusters[clsIdx])
      modIdx = 0
      setActiveTab(nocModules[0])
    }, nocInterval * 60_000)
    return () => { clearInterval(modTimer); clearInterval(clsTimer) }
  }, [nocMode, nocInterval, nocModules.join(','), clusters.join(',')])

  useEffect(() => {
    if (activeTab === 'admin') {
      axios.get('/api/docker/hosts').then(({ data }) => setDockerHosts(data || [])).catch(() => {})
      if (isAdmin) {
        loadWebhooks()
        loadThresholds()
        fetchAudit()
      }
    }
  }, [activeTab])

  useEffect(() => {
    if (activeTab === 'namespaces' && pods.length === 0 && cluster) {
      setLoading(true)
      axios.get(`/api/resources?${cp()}`)
        .then(({ data }) => setPods(data || []))
        .catch(() => {})
        .finally(() => setLoading(false))
    }
  }, [activeTab])

  async function fetchPods() {
    setLoading(true); setError('')
    try {
      const { data } = await axios.get(`/api/resources?${cp()}${namespace ? `&namespace=${namespace}` : ''}`)
      setPods(data || [])
    } catch { setError(t('errorApiConsult')) }
    finally { setLoading(false) }
  }

  // Local date as YYYY-MM-DD
  function localDateStr(d) {
    const pad = n => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
  }

  // Convert YYYY-MM-DD to ISO timestamp at local midnight (start of that day in local tz)
  function localDayStart(dateStr) {
    const [y, m, d] = dateStr.split('-').map(Number)
    return new Date(y, m - 1, d, 0, 0, 0, 0).toISOString()
  }

  // Convert YYYY-MM-DD to ISO timestamp at next local midnight (exclusive end)
  function localDayEnd(dateStr) {
    const [y, m, d] = dateStr.split('-').map(Number)
    return new Date(y, m - 1, d + 1, 0, 0, 0, 0).toISOString()
  }

  function histParams() {
    const p = new URLSearchParams()
    if (cluster)   p.set('cluster',   cluster)
    if (histNs)    p.set('namespace', histNs)
    if (histStart) p.set('start', histStart.length === 10 ? localDayStart(histStart) : histStart)
    if (histEnd)   p.set('end',   localDayEnd(histEnd.slice(0, 10)))
    return p.toString()
  }

  const histPresetLabels = t('histPresets') || {}
  const HIST_PRESETS = [
    { id: 'today', label: histPresetLabels.today || 'Hoje' },
    { id: '12h',   label: histPresetLabels['12h'] || 'Últimas 12h' },
    { id: '24h',   label: histPresetLabels['24h'] || 'Últimas 24h' },
    { id: '7d',    label: histPresetLabels['7d'] || '7 dias' },
  ]

  async function doFetchHistory(start, end) {
    setHistLoading(true); setHistError('')
    try {
      const p = new URLSearchParams()
      if (cluster) p.set('cluster', cluster)
      if (histNs)  p.set('namespace', histNs)
      // date-only strings → convert to local timezone boundaries
      if (start) p.set('start', start.length === 10 ? localDayStart(start) : start)
      if (end)   p.set('end',   localDayEnd(end.slice(0, 10)))
      const { data } = await axios.get(`/api/history?${p}`, { headers: { Authorization: `Bearer ${user.token}` } })
      setHistory(data || [])
    } catch { setHistError(t('errorFetchingHistory')) }
    finally { setHistLoading(false) }
  }

  async function applyPreset(id) {
    const now = new Date()
    setActivePreset(id)
    let start = '', end = ''
    if (id === 'today') {
      start = localDateStr(now)
      end   = start
    } else if (id === '12h') {
      start = new Date(now - 12 * 3600 * 1000).toISOString()
    } else if (id === '24h') {
      start = new Date(now - 24 * 3600 * 1000).toISOString()
    }
    // '7d' leaves start/end empty → backend default (7 days)
    setHistStart(start); setHistEnd(end)
    await doFetchHistory(start, end)
  }

  function onHistDateChange(setter, value) {
    setter(value); setActivePreset(null)
  }

  async function fetchHistory() {
    await doFetchHistory(histStart, histEnd)
  }

  async function exportHistoryCSV() {
    try {
      const resp = await axios.get(`/api/history/csv?${histParams()}`, {
        headers: { Authorization: `Bearer ${user.token}` },
        responseType: 'blob',
      })
      const disposition = resp.headers['content-disposition'] || ''
      const match = disposition.match(/filename="?([^"]+)"?/)
      const fname = match ? match[1] : 'historico.csv'
      const url = URL.createObjectURL(resp.data)
      const a = document.createElement('a')
      a.href = url; a.download = fname; a.click()
      URL.revokeObjectURL(url)
    } catch { setHistError(t('errorExportingCsv')) }
  }

  async function fetchNodes() {
    setNodesLoad(true); setNodesError('')
    try {
      const { data } = await axios.get(`/api/nodes?${cp()}`)
      setNodes(data || [])
    } catch { setNodesError(t('errorFetchingNodes')) }
    finally { setNodesLoad(false) }
  }

  // admin cluster handlers
  async function adminValidate() {
    setAdminLoading(true); setAdminMsg({ type: '', text: '' })
    try {
      const { data } = await axios.post('/api/admin/validate', { kubeconfig: adminKubeconfig })
      if (data.error) { setAdminMsg({ type: 'error', text: data.error }); return }
      setAdminContexts(data.contexts || [])
      setAdminSelCtx(data.contexts?.[0] || '')
      setAdminStep(2)
      setAdminMsg({ type: 'ok', text: t('contextsFound')(data.contexts.length) })
    } catch { setAdminMsg({ type: 'error', text: t('errorValidatingKubeconfig') }) }
    finally { setAdminLoading(false) }
  }

  async function adminCreateSA() {
    setAdminLoading(true); setAdminMsg({ type: '', text: '' })
    try {
      const { data } = await axios.post('/api/admin/create-sa', { kubeconfig: adminKubeconfig, context: adminSelCtx, namespace: adminNs })
      if (data.error) { setAdminMsg({ type: 'error', text: data.error }); return }
      setAdminSAKubeconfig(data.kubeconfig)
      setAdminStep(3)
      setAdminMsg({ type: 'ok', text: data.message + ' ' + t('kubeconfigGenerated') })
    } catch { setAdminMsg({ type: 'error', text: t('errorCreatingSA') }) }
    finally { setAdminLoading(false) }
  }

  async function adminApply() {
    setAdminLoading(true); setAdminMsg({ type: '', text: '' })
    try {
      const { data } = await axios.post('/api/admin/apply', { kubeconfig: adminSAKubeconfig || adminKubeconfig })
      if (data.error) { setAdminMsg({ type: 'error', text: data.error }); return }
      setClusters(data.clusters || [])
      if (data.clusters?.length > 0 && !cluster) setCluster(data.clusters[0])
      setAdminStep(1); setAdminKubeconfig(''); setAdminSAKubeconfig(''); setAdminContexts([])
      setAdminMsg({ type: 'ok', text: data.message })
    } catch { setAdminMsg({ type: 'error', text: t('errorApplyingKubeconfig') }) }
    finally { setAdminLoading(false) }
  }

  async function adminDeleteCluster() {
    setDelClusterLoading(true); setDelClusterErr('')
    try {
      const { data } = await axios.delete('/api/admin/cluster/delete', {
        data: { name: delClusterModal, password: delClusterPwd }
      })
      const list = data.clusters || []
      setClusters(list)
      if (cluster === delClusterModal) setCluster(list[0] || '')
      setDelClusterModal(null); setDelClusterPwd('')
      setAdminMsg({ type: 'ok', text: t('clusterRemoved')(delClusterModal) })
    } catch (e) {
      setDelClusterErr(e.response?.status === 403 ? t('wrongPassword') : t('errorRemovingCluster'))
    } finally { setDelClusterLoading(false) }
  }

  function adminHandleFile(e) {
    const file = e.target.files[0]; if (!file) return
    const reader = new FileReader()
    reader.onload = evt => { setAdminKubeconfig(evt.target.result); setAdminStep(1); setAdminContexts([]) }
    reader.readAsText(file)
    e.target.value = ''
  }

  // monitor computed
  const rows = []; let cpuAlerts = 0, memAlerts = 0
  pods.forEach(pod => pod.containers.forEach(c => {
    const cpuPct = calcPct(c.cpu_usage, c.cpu_limit, parseCPU)
    const memPct = calcPct(c.memory_usage, c.memory_limit, parseMem)
    const cpuAlert = typeof cpuPct === 'number' && cpuPct >= 90
    const memAlert = typeof memPct === 'number' && memPct >= 90
    const any = cpuAlert || memAlert || (typeof cpuPct === 'number' && cpuPct >= 85) || (typeof memPct === 'number' && memPct >= 85)
    if (cpuAlert) cpuAlerts++
    if (memAlert) memAlerts++
    if (filter === 'alerts' && !any) return
    rows.push({ pod, c, cpuPct, memPct, cpuAlert, memAlert, any })
  }))

  // top 10 — estado independente
  const [top10Cluster,  setTop10Cluster]  = useState('')
  const [top10Pods,     setTop10Pods]     = useState([])
  const [top10Loading,  setTop10Loading]  = useState(false)
  const [top10Error,    setTop10Error]    = useState('')

  useEffect(() => {
    if (activeTab !== 'top10') return
    if (!top10Cluster && cluster) setTop10Cluster(cluster)
  }, [activeTab, cluster])

  useEffect(() => {
    if (!top10Cluster) return
    setTop10Loading(true); setTop10Error('')
    axios.get(`/api/resources?cluster=${top10Cluster}`)
      .then(({ data }) => setTop10Pods(data || []))
      .catch(() => setTop10Error(t('errorApiConsult')))
      .finally(() => setTop10Loading(false))
  }, [top10Cluster])

  const topOffenders = useMemo(() =>
    top10Pods
      .flatMap(pod => pod.containers.map(c => ({
        ...c,
        podName:   pod.pod,
        namespace: pod.namespace,
        node:      pod.node,
        cpuPct:    calcPct(c.cpu_usage,    c.cpu_limit,    parseCPU),
        memPct:    calcPct(c.memory_usage, c.memory_limit, parseMem),
      })))
      .filter(c => typeof c.cpuPct === 'number' || typeof c.memPct === 'number')
      .sort((a, b) => {
        const wa = Math.max(typeof a.cpuPct === 'number' ? a.cpuPct : 0, typeof a.memPct === 'number' ? a.memPct : 0)
        const wb = Math.max(typeof b.cpuPct === 'number' ? b.cpuPct : 0, typeof b.memPct === 'number' ? b.memPct : 0)
        return wb - wa
      })
      .slice(0, 15),
    [top10Pods])

  // flatContainers — usado pelo Monitor e aba Namespaces
  const flatContainers = useMemo(() =>
    pods.flatMap(pod => pod.containers.map(c => ({
      ...c,
      podName:  pod.pod,
      namespace: pod.namespace,
      node:     pod.node,
      cpuPct:   calcPct(c.cpu_usage, c.cpu_limit,    parseCPU),
      memPct:   calcPct(c.memory_usage, c.memory_limit, parseMem),
    }))), [pods])

  // namespaces aggregation
  const nsAggregated = useMemo(() => {
    const map = new Map()
    flatContainers.forEach(c => {
      const ns = c.namespace
      if (!map.has(ns)) map.set(ns, { ns, pods: new Set(), containers: 0, cpuUsage: 0, memUsage: 0, alerts: 0, cautions: 0 })
      const row = map.get(ns)
      row.pods.add(c.podName)
      row.containers++
      if (typeof c.cpuPct === 'number' && c.cpuPct >= 90) row.alerts++
      else if (typeof c.cpuPct === 'number' && c.cpuPct >= 85) row.cautions++
      if (typeof c.memPct === 'number' && c.memPct >= 90) row.alerts++
      else if (typeof c.memPct === 'number' && c.memPct >= 85) row.cautions++
      const cpuUsed = parseCPU(c.cpu_usage)
      const memUsed = parseMem(c.memory_usage)
      if (cpuUsed) row.cpuUsage += cpuUsed
      if (memUsed) row.memUsage += memUsed
    })
    return [...map.values()]
      .map(r => ({ ...r, pods: r.pods.size }))
      .sort((a, b) => b.alerts - a.alerts || b.cautions - a.cautions || a.ns.localeCompare(b.ns))
  }, [flatContainers])

  // storage
  const [storage,      setStorage]      = useState([])
  const [storageLoad,  setStorageLoad]  = useState(false)
  const [storageError, setStorageError] = useState('')

  async function fetchStorage() {
    setStorageLoad(true); setStorageError('')
    try {
      const { data } = await axios.get('/api/storage', { headers: { Authorization: `Bearer ${user.token}` }, params: { cluster } })
      setStorage(data || [])
    } catch (err) {
      setStorageError(err.response?.data?.error || err.response?.data || t('errorFetchingStorage'))
    } finally { setStorageLoad(false) }
  }

  // docker/podman
  const [dockerHosts,      setDockerHosts]      = useState([])
  const [dockerHost,       setDockerHost]       = useState('')
  const [dockerContainers, setDockerContainers] = useState([])
  const [dockerLoad,       setDockerLoad]       = useState(false)
  const [dockerError,      setDockerError]      = useState('')
  const [dockerFilter,     setDockerFilter]     = useState('all')

  const [clusterContainers,     setClusterContainers]     = useState([])
  const [clusterContainersLoad, setClusterContainersLoad] = useState(false)
  const [clusterContainersErr,  setClusterContainersErr]  = useState('')
  const [clusterContainerHost,  setClusterContainerHost]  = useState('')
  const [clusterFilter,         setClusterFilter]         = useState('all')

  const [helmReleases,    setHelmReleases]    = useState([])
  const [helmLoad,        setHelmLoad]        = useState(false)
  const [helmError,       setHelmError]       = useState('')

  const [deployments,     setDeployments]     = useState([])
  const [deploymentsLoad, setDeploymentsLoad] = useState(false)
  const [deploymentsError,setDeploymentsError]= useState('')

  useEffect(() => {
    if (activeTab === 'helm' && cluster) fetchHelmReleases()
    if (activeTab === 'deployments' && cluster) fetchDeployments()
  }, [activeTab, cluster])

  async function fetchDeployments() {
    setDeploymentsLoad(true); setDeploymentsError('')
    try {
      const { data } = await axios.get('/api/deployments', { params: { cluster } })
      setDeployments(data || [])
    } catch (err) {
      setDeploymentsError(err.response?.data || t('errorFetchingDeployments'))
    } finally { setDeploymentsLoad(false) }
  }

  async function fetchHelmReleases() {
    setHelmLoad(true); setHelmError('')
    try {
      const { data } = await axios.get('/api/helm/releases', { params: { cluster } })
      setHelmReleases(data || [])
    } catch (err) {
      setHelmError(err.response?.data || t('errorFetchingHelm'))
    } finally { setHelmLoad(false) }
  }

  // hosts filtrados por tipo
  const externalHosts = dockerHosts.filter(h => !h.cluster)
  const clusterHosts  = dockerHosts.filter(h => h.cluster)

  useEffect(() => {
    if (activeTab !== 'docker' && activeTab !== 'containers') return
    axios.get('/api/docker/hosts').then(({ data }) => {
      const hosts = data || []
      setDockerHosts(hosts)
      const ext = hosts.filter(h => !h.cluster)
      const cls = hosts.filter(h => h.cluster)
      if (!dockerHost && ext.length > 0) setDockerHost(ext[0].name)
      if (!clusterContainerHost && cls.length > 0) setClusterContainerHost(cls[0].name)
      if (activeTab === 'docker' && ext.length > 0) {
        fetchDockerContainersFromHosts(ext.map(h => h.name))
      }
    }).catch(() => {})
    if (activeTab === 'containers' && cluster) {
      fetchClusterContainers()
    }
  }, [activeTab])

  async function fetchDockerContainersFromHosts(hostNames) {
    setDockerLoad(true); setDockerError('')
    try {
      const results = await Promise.all(
        hostNames.map(h =>
          axios.get('/api/docker/containers', { params: { host: h } })
            .then(({ data }) => (data || []).map(c => ({ ...c, source_host: h })))
            .catch(() => [])
        )
      )
      setDockerContainers(results.flat())
    } catch (err) {
      setDockerError(t('errorFetchingDocker'))
    } finally { setDockerLoad(false) }
  }

  async function fetchDockerContainers() {
    await fetchDockerContainersFromHosts(externalHosts.map(h => h.name))
  }

  async function fetchClusterContainers() {
    setClusterContainersLoad(true); setClusterContainersErr('')
    try {
      const params = cluster ? { cluster } : { host: clusterContainerHost }
      const { data } = await axios.get('/api/containers', { params })
      setClusterContainers(data || [])
    } catch (err) {
      setClusterContainersErr(err.response?.data || t('errorFetchingContainers'))
    } finally { setClusterContainersLoad(false) }
  }

  function formatBytes(bytes) {
    if (!bytes || bytes <= 0) return '-'
    if (bytes >= 1024 ** 3) return (bytes / 1024 ** 3).toFixed(1) + ' GiB'
    if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(0) + ' MiB'
    return (bytes / 1024).toFixed(0) + ' KiB'
  }

  // logs
  const [logCluster,    setLogCluster]    = useState('')
  const [logNsList,     setLogNsList]     = useState([])
  const [logNamespace,  setLogNamespace]  = useState('')
  const [logPod,        setLogPod]        = useState('')
  const [logContainer,  setLogContainer]  = useState('')
  const [logTail,       setLogTail]       = useState('200')
  const [logPods,       setLogPods]       = useState([])
  const [logContainers, setLogContainers] = useState([])
  const [logContent,    setLogContent]    = useState('')
  const [logLoad,       setLogLoad]       = useState(false)
  const [logError,      setLogError]      = useState('')

  useEffect(() => {
    if (activeTab !== 'logs') return
    if (!logCluster && cluster) setLogCluster(cluster)
  }, [activeTab, cluster])

  useEffect(() => {
    if (!logCluster) { setLogNsList([]); setLogNamespace(''); return }
    axios.get('/api/namespaces', { params: { cluster: logCluster } })
      .then(r => setLogNsList(r.data || []))
      .catch(() => setLogNsList([]))
    setLogNamespace(''); setLogPod(''); setLogPods([]); setLogContent('')
  }, [logCluster])

  async function fetchLogPods() {
    if (!logNamespace) return
    try {
      const { data } = await axios.get('/api/resources', { params: { cluster: logCluster, namespace: logNamespace } })
      setLogPods(data || [])
      setLogPod(''); setLogContainer(''); setLogContent('')
    } catch { setLogPods([]) }
  }

  useEffect(() => {
    if (!logPod) { setLogContainers([]); setLogContainer(''); return }
    const pod = logPods.find(p => p.pod === logPod)
    const ctrs = pod?.containers?.map(c => c.name) || []
    setLogContainers(ctrs)
    setLogContainer(ctrs[0] || '')
  }, [logPod])

  async function fetchLogs() {
    setLogLoad(true); setLogError(''); setLogContent('')
    try {
      const { data } = await axios.get('/api/logs', {
        params: { cluster: logCluster, namespace: logNamespace, pod: logPod, container: logContainer, tail: logTail }
      })
      setLogContent(data || t('noLogs'))
    } catch (err) {
      setLogError(err.response?.data || t('errorFetchingLogs'))
    } finally { setLogLoad(false) }
  }

  // topology
  const [topoData,        setTopoData]        = useState(null)
  const [topoNs,          setTopoNs]          = useState('')
  const [topoNsFilter,    setTopoNsFilter]     = useState('')
  const [topoLoad,        setTopoLoad]        = useState(false)
  const [topoError,       setTopoError]       = useState('')
  const [topoAutoRefresh, setTopoAutoRefresh] = useState(true)
  const [topoSseConn,     setTopoSseConn]     = useState(false)

  const topoNsFiltered = (() => {
    if (!topoNsFilter) return namespaces
    try { const re = new RegExp(topoNsFilter, 'i'); return namespaces.filter(ns => re.test(ns)) }
    catch { return namespaces }
  })()

  async function fetchTopology() {
    if (!cluster) return
    setTopoLoad(true); setTopoError('')
    try {
      const params = { cluster }
      if (topoNs) params.namespace = topoNs
      const { data } = await axios.get('/api/topology', { params })
      setTopoData(data)
    } catch { setTopoError(t('topoError')) }
    finally { setTopoLoad(false) }
  }

  // SSE auto-refresh para topologia
  useEffect(() => {
    if (activeTab !== 'topology' || !topoAutoRefresh || !topoData || !cluster) return
    const url = `/api/sse/events?cluster=${encodeURIComponent(cluster)}`
    const es = new EventSource(url)
    es.onopen = () => setTopoSseConn(true)
    es.addEventListener('topology_refresh', e => {
      try {
        const d = JSON.parse(e.data)
        if (!d.clusters || d.clusters.includes(cluster)) {
          // re-fetch silencioso: não mostra spinner
          const params = { cluster }
          if (topoNs) params.namespace = topoNs
          axios.get('/api/topology', { params })
            .then(res => setTopoData(res.data))
            .catch(() => {})
        }
      } catch {}
    })
    es.onerror = () => { setTopoSseConn(false); es.close() }
    return () => { es.close(); setTopoSseConn(false) }
  }, [activeTab, topoAutoRefresh, !!topoData, cluster])

  // quotas
  const [quotasData,   setQuotasData]   = useState([])
  const [quotasLoad,   setQuotasLoad]   = useState(false)
  const [quotasError,  setQuotasError]  = useState('')

  async function fetchQuotas() {
    if (!cluster) return
    setQuotasLoad(true); setQuotasError('')
    try {
      const { data } = await axios.get('/api/quotas', { params: { cluster } })
      setQuotasData(data || [])
    } catch (err) {
      const msg = err.response?.data || err.message || t('quotasError')
      setQuotasError(typeof msg === 'string' ? msg : JSON.stringify(msg))
    }
    finally { setQuotasLoad(false) }
  }

  // audit
  const [auditData,    setAuditData]    = useState([])
  const [auditLoad,    setAuditLoad]    = useState(false)
  const [auditError,   setAuditError]   = useState('')
  const [auditLimit,   setAuditLimit]   = useState(100)

  async function fetchAudit() {
    setAuditLoad(true); setAuditError('')
    try {
      const { data } = await axios.get(`/api/audit?limit=${auditLimit}`)
      setAuditData(data || [])
    } catch { setAuditError(t('auditError')) }
    finally { setAuditLoad(false) }
  }

  // webhooks
  const [webhooks,     setWebhooks]     = useState([])
  const [whLoad,       setWhLoad]       = useState(false)
  const [whMsg,        setWhMsg]        = useState({ type: '', text: '' })
  const [whForm,       setWhForm]       = useState({ id: 0, name: '', url: '', events: 'critical', enabled: true })

  async function loadWebhooks() {
    try { const { data } = await axios.get('/api/webhooks'); setWebhooks(data || []) } catch {}
  }

  async function saveWebhook() {
    setWhLoad(true); setWhMsg({ type: '', text: '' })
    try {
      await axios.post('/api/webhooks', whForm)
      setWhMsg({ type: 'ok', text: t('webhookSaved') })
      setWhForm({ id: 0, name: '', url: '', events: 'critical', enabled: true })
      loadWebhooks()
    } catch (err) {
      setWhMsg({ type: 'error', text: err.response?.data?.error || t('webhookError') })
    } finally { setWhLoad(false) }
  }

  async function deleteWebhook(id) {
    try {
      await axios.delete(`/api/webhooks?id=${id}`)
      setWhMsg({ type: 'ok', text: t('webhookDeleted') })
      loadWebhooks()
    } catch {}
  }

  async function testWebhook(url) {
    setWhMsg({ type: '', text: '' })
    try {
      const { data } = await axios.post('/api/webhooks/test', { url })
      setWhMsg({ type: 'ok', text: `Teste enviado! Status HTTP: ${data.status}` })
    } catch (err) {
      const msg = err.response?.data || err.message
      setWhMsg({ type: 'error', text: `Erro no teste: ${typeof msg === 'string' ? msg : JSON.stringify(msg)}` })
    }
  }

  // thresholds
  const [thresholds,   setThresholds]   = useState([])
  const [thrLoad,      setThrLoad]      = useState(false)
  const [thrMsg,       setThrMsg]       = useState({ type: '', text: '' })
  const [thrForm,      setThrForm]      = useState({ id: 0, cluster: '', namespace: '', warn_pct: 85, crit_pct: 90 })

  async function loadThresholds() {
    try { const { data } = await axios.get('/api/thresholds'); setThresholds(data || []) } catch {}
  }

  async function saveThreshold() {
    setThrLoad(true); setThrMsg({ type: '', text: '' })
    try {
      await axios.post('/api/thresholds', thrForm)
      setThrMsg({ type: 'ok', text: t('thresholdSaved') })
      setThrForm({ id: 0, cluster: '', namespace: '', warn_pct: 85, crit_pct: 90 })
      loadThresholds()
    } catch (err) {
      setThrMsg({ type: 'error', text: err.response?.data || t('thresholdError') })
    } finally { setThrLoad(false) }
  }

  async function deleteThreshold(id) {
    try {
      await axios.delete(`/api/thresholds?id=${id}`)
      setThrMsg({ type: 'ok', text: t('thresholdDeleted') })
      loadThresholds()
    } catch {}
  }

  // session timeout warning
  const [sessionWarning, setSessionWarning] = useState(null) // minutos restantes

  useEffect(() => {
    if (!user) return
    const stored = localStorage.getItem('pm_token')
    if (!stored) return
    // Decodifica o JWT para pegar exp (não valida assinatura, apenas lê)
    try {
      const parts = stored.split('.')
      if (parts.length < 1) return
      const payload = JSON.parse(atob(parts[0]))
      const expMs = payload.exp * 1000
      const checkRemaining = () => {
        const remaining = Math.floor((expMs - Date.now()) / 60000)
        if (remaining <= 0) { logout(); return }
        if (remaining <= 30) setSessionWarning(remaining)
        else setSessionWarning(null)
      }
      checkRemaining()
      const interval = setInterval(checkRemaining, 60000)
      return () => clearInterval(interval)
    } catch {}
  }, [user])

  // orphans
  const emptyOrphans = { pvcs: [], services: [], config_maps: [], secrets: [], ingresses: [], service_accounts: [] }
  const [orphans,      setOrphans]      = useState(emptyOrphans)
  const [orphansLoad,  setOrphansLoad]  = useState(false)
  const [orphansError, setOrphansError] = useState('')
  const [orphansExclude, setOrphansExclude] = useState('')

  async function fetchOrphans() {
    setOrphansLoad(true); setOrphansError('')
    try {
      const params = { cluster }
      if (orphansExclude.trim()) params.exclude = orphansExclude.trim()
      const { data } = await axios.get('/api/orphans', { headers: { Authorization: `Bearer ${user.token}` }, params })
      setOrphans(data || emptyOrphans)
    } catch (err) {
      setOrphansError(err.response?.data?.error || err.response?.data || t('errorFetchingOrphans'))
    } finally { setOrphansLoad(false) }
  }

  // histórico
  const histSessions = useMemo(() => {
    const map = new Map()
    history.forEach(rec => {
      if (!map.has(rec.session_id)) map.set(rec.session_id, { id: rec.session_id, at: rec.captured_at, records: [] })
      map.get(rec.session_id).records.push(rec)
    })
    return [...map.values()]
  }, [history])
  const sessionsAsc = useMemo(() => [...histSessions].reverse(), [histSessions])
  const trendMap = useMemo(() => {
    const m = {}
    sessionsAsc.forEach((session, idx) => {
      if (idx === 0) return
      const prev = sessionsAsc[idx - 1]; const prevIndex = {}
      prev.records.forEach(r => { prevIndex[`${r.namespace}/${r.pod}:${r.container}`] = r })
      session.records.forEach(r => {
        const key = `${r.namespace}/${r.pod}:${r.container}`
        const p = prevIndex[key]; if (!p) return
        const cpuDiff = parseCPU(r.cpu_usage) - parseCPU(p.cpu_usage)
        const memDiff = parseMem(r.memory_usage) - parseMem(p.memory_usage)
        m[`${session.id}:${key}`] = { cpu: cpuDiff > 0 ? 'up' : cpuDiff < 0 ? 'down' : 'same', mem: memDiff > 0 ? 'up' : memDiff < 0 ? 'down' : 'same' }
      })
    })
    return m
  }, [sessionsAsc])

  if (!user) return <LoginPage onLogin={handleLogin} lang={lang} />

  if (currentView === 'users' && isAdmin)
    return <UserManagementPage onBack={() => setCurrentView('main')} lang={lang} />

  return (
    <div className='app'>

      {helpOpen && <HelpModal role={user.role} onClose={() => setHelpOpen(false)} lang={lang} />}

      <header className='app-header'>
        <div className='app-header-logo'>
          {/* Pod isométrico 3D */}
          <svg viewBox='0 0 24 24' fill='none'>
            {/* Face superior — mais clara (luz de cima) */}
            <path d='M12 2.5 L21.5 7.5 L12 12.5 L2.5 7.5 Z'
              fill='rgba(255,255,255,0.28)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
            {/* Face esquerda — tom médio */}
            <path d='M2.5 7.5 L2.5 16 L12 21 L12 12.5 Z'
              fill='rgba(255,255,255,0.12)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
            {/* Face direita — mais escura (sombra) */}
            <path d='M21.5 7.5 L21.5 16 L12 21 L12 12.5 Z'
              fill='rgba(0,0,0,0.18)' stroke='rgba(255,255,255,0.85)' strokeWidth='1.1'/>
            {/* Containers na face superior: 3 círculos representando os containers do pod */}
            <circle cx='8.8'  cy='7.2'  r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
            <circle cx='15.2' cy='7.2'  r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
            <circle cx='12'   cy='10.2' r='1.15' fill='rgba(255,255,255,0.9)' stroke='none'/>
          </svg>
        </div>
        <div className='app-header-text'>
          <h1>Pod Resource Monitor</h1>
        </div>

        <div className='app-header-right'>
          {clusters.length > 1 ? (
            <select className='cluster-select' value={cluster} onChange={e => setCluster(e.target.value)}>
              {clusters.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          ) : (
            <span className='app-header-tag'>{cluster || 'local'}</span>
          )}

          {nocMode && (
            <span className='noc-badge' title={`NOC • ${nocInterval}min/cluster • ${nocModules.join(', ')}`}>
              {t('nocBadge')}
            </span>
          )}

          <div className='header-user'>
            <span className='header-username'>{user.username}</span>
            <span className={`header-role ${user.role === 'administration' ? 'admin' : user.role === 'dev' ? 'dev' : 'reader'}`}>
              {user.role === 'administration' ? 'Admin' : user.role === 'dev' ? 'Dev' : 'Reader'}
            </span>
          </div>

          {isAdmin && (
            <div className='gear-wrap' ref={gearRef}>
              <button className='gear-btn' onClick={() => setGearOpen(o => !o)} title={t('settingsBtn')}>
                <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
                  <circle cx='12' cy='12' r='3'/>
                  <path d='M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z'/>
                </svg>
              </button>
              {gearOpen && (
                <div className='gear-dropdown'>
                  <button onClick={() => { setCurrentView('users'); setGearOpen(false) }}>
                    <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
                      <path d='M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2'/>
                      <circle cx='9' cy='7' r='4'/>
                      <path d='M23 21v-2a4 4 0 0 0-3-3.87'/>
                      <path d='M16 3.13a4 4 0 0 1 0 7.75'/>
                    </svg>
                    {t('manageUsersMenu')}
                  </button>
                </div>
              )}
            </div>
          )}

          <button className='help-btn' onClick={() => setHelpOpen(true)} title={t('helpBtn')}>
            <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
              <circle cx='12' cy='12' r='10'/>
              <path d='M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3'/>
              <line x1='12' y1='17' x2='12.01' y2='17' strokeWidth='3' strokeLinecap='round'/>
            </svg>
          </button>

          <button
            className='theme-toggle'
            onClick={() => setLang(l => l === 'pt' ? 'en' : 'pt')}
            title='Switch language / Trocar idioma'
            style={{ fontSize: 12, fontWeight: 700, letterSpacing: '0.04em', minWidth: 34 }}>
            {lang === 'pt' ? 'EN' : 'PT'}
          </button>

          <button
            className='theme-toggle'
            onClick={() => setNavLayout(l => l === 'top' ? 'sidebar' : 'top')}
            title={navLayout === 'top' ? t('navLayoutSidebar') : t('navLayoutTop')}>
            {navLayout === 'top' ? (
              <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
                <rect x='3' y='4' width='7' height='16' rx='1.5'/>
                <line x1='14' y1='7' x2='21' y2='7'/>
                <line x1='14' y1='12' x2='21' y2='12'/>
                <line x1='14' y1='17' x2='21' y2='17'/>
              </svg>
            ) : (
              <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
                <rect x='3' y='3' width='18' height='4' rx='1.5'/>
                <line x1='6' y1='11' x2='18' y2='11'/>
                <line x1='6' y1='15' x2='18' y2='15'/>
                <line x1='6' y1='19' x2='18' y2='19'/>
              </svg>
            )}
          </button>

          <div className='theme-picker-wrap' ref={themeRef}>
            <button className='theme-toggle' onClick={() => setThemeOpen(o => !o)} title={t('themesBtn')}>
              <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
                <circle cx='12' cy='12' r='10'/>
                <path d='M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z'/>
                <path d='M2 12h20'/>
              </svg>
            </button>
            {themeOpen && (
              <div className='theme-picker-dropdown'>
                {THEMES.map(t => (
                  <button key={t.id} className={`theme-option ${theme === t.id ? 'active' : ''}`}
                    onClick={() => applyTheme(t.id)}>
                    <span className='theme-swatch' style={{ background: t.color }} />
                    {t.label}
                    {theme === t.id && <span className='theme-check'>✓</span>}
                  </button>
                ))}
              </div>
            )}
          </div>

          <button className='logout-btn' onClick={logout} title={t('logoutBtn')}>
            <svg viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
              <path d='M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4'/>
              <polyline points='16 17 21 12 16 7'/>
              <line x1='21' y1='12' x2='9' y2='12'/>
            </svg>
          </button>
        </div>
      </header>

      {sessionWarning !== null && (
        <div style={{
          background: 'var(--caution-bg, #7c3a00)', color: 'var(--caution-text, #fbbf24)',
          borderBottom: '1px solid var(--caution-border, #f59e0b)',
          padding: '6px 16px', fontSize: 13, display: 'flex', alignItems: 'center', gap: 12,
        }}>
          <span>⚠ {t('sessionExpiringSoon')(sessionWarning)}</span>
          <button onClick={logout} style={{
            fontSize: 12, padding: '2px 10px', borderRadius: 4, border: '1px solid currentColor',
            background: 'transparent', color: 'inherit', cursor: 'pointer',
          }}>{t('sessionRenew')}</button>
        </div>
      )}

      <div className={`app-body ${navLayout === 'sidebar' ? 'sidebar-layout' : ''}`}>
        <nav className={navLayout === 'sidebar' ? 'tabs-sidebar' : 'tabs'}>
          {visibleTabs.map(t => (
            <button key={t.id} className={`tab-btn ${activeTab === t.id ? 'active' : ''}`} onClick={() => setActiveTab(t.id)}>
              {t.label}
            </button>
          ))}
        </nav>

      <main className='app-main'>

        {/* ── MONITOR ──────────────────────────────────────────────────── */}
        {activeTab === 'monitor' && (
          <>
            <div className='controls'>
              <select value={namespace} onChange={e => setNamespace(e.target.value)}>
                <option value=''>{t('allNamespacesOpt')}</option>
                {namespaces.map(ns => <option key={ns} value={ns}>{ns}</option>)}
              </select>
              <select value={filter} onChange={e => setFilter(e.target.value)}>
                <option value='all'>{t('allPodsOpt')}</option>
                <option value='alerts'>{t('onlyAlertsOpt')}</option>
              </select>
              <button onClick={fetchPods} disabled={loading}>{loading ? t('consultingBtn') : t('refreshBtn')}</button>
            </div>
            {error && <div className='error-box'>{error}</div>}
            {pods.length > 0 && (
              <div className='metrics'>
                <MetricCard label={t('totalPods')} value={pods.length} />
                <MetricCard label={t('containers')} value={rows.length} />
                <MetricCard label='CPU > 90%' value={cpuAlerts} alert={cpuAlerts > 0} />
                <MetricCard label='Mem > 90%' value={memAlerts} alert={memAlerts > 0} />
              </div>
            )}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>{t('thPod')}</th><th>{t('thNamespace')}</th><th>{t('thNode')}</th><th>{t('thContainer')}</th>
                  <th>{t('thCpuReq')}</th><th>{t('thCpuLimit')}</th>
                  <th>{t('thMemReq')}</th><th>{t('thMemLimit')}</th>
                  <th>{t('thCpuUsage')}</th><th>{t('thCpuPct')}</th>
                  <th>{t('thMemUsage')}</th><th>{t('thMemPct')}</th>
                  <th>{t('thStatus')}</th>
                </tr></thead>
                <tbody>
                  {rows.length === 0 && (
                    <tr><td colSpan={13} className='empty'>
                      {loading ? t('loadingData') : pods.length === 0 ? t('noPodFound') : t('noResults')}
                    </td></tr>
                  )}
                  {rows.map(({ pod, c, cpuPct, memPct, cpuAlert, memAlert, any }, i) => {
                    const isRunning = pod.phase === 'Running'
                    const isPending = pod.phase === 'Pending'
                    const isSucceeded = pod.phase === 'Succeeded'
                    const phaseWarn = !isRunning && !isPending && !isSucceeded
                    const badgeCls = isSucceeded
                      ? 'badge ok'
                      : phaseWarn
                        ? 'badge warn'
                        : isPending
                          ? 'badge caution'
                          : (cpuAlert || memAlert ? 'badge warn' : 'badge ok')
                    return (
                    <tr key={i} className={phaseWarn || (!isSucceeded && (cpuAlert || memAlert)) ? 'alert-row' : ''}>
                      <td>{pod.pod}</td><td>{pod.namespace}</td><td>{pod.node}</td><td>{c.name}</td>
                      <td>{c.cpu_request    || '-'}</td>
                      <td>{c.cpu_limit      || '-'}</td>
                      <td>{c.memory_request || '-'}</td>
                      <td>{c.memory_limit   || '-'}</td>
                      <td>{c.cpu_usage    || '-'}</td>
                      <td className={pctClass(cpuPct)}>{fmtPct(cpuPct)}</td>
                      <td>{c.memory_usage || '-'}</td>
                      <td className={pctClass(memPct)}>{fmtPct(memPct)}</td>
                      <td><span className={badgeCls}>{pod.phase || 'Unknown'}</span></td>
                    </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── TOP 10 ───────────────────────────────────────────────────── */}
        {activeTab === 'top10' && (
          <>
            <div className='controls'>
              <select value={top10Cluster} onChange={e => setTop10Cluster(e.target.value)}>
                <option value=''>{t('clusterPlaceholder')}</option>
                {clusters.map(c => <option key={c} value={c}>{c}</option>)}
              </select>
              <button
                disabled={!top10Cluster || top10Loading}
                onClick={() => { const c = top10Cluster; setTop10Cluster(''); setTimeout(() => setTop10Cluster(c), 0) }}
              >{top10Loading ? t('consultingBtn') : t('refreshBtn')}</button>
            </div>
            {top10Error && <div className='error-box'>{top10Error}</div>}
            {top10Loading ? (
              <div className='table-wrap'><div className='empty'>{t('loadingData')}</div></div>
            ) : !top10Cluster ? (
              <div className='table-wrap'><div className='empty'>{t('selectClusterForAnalysis')}</div></div>
            ) : top10Pods.length === 0 ? (
              <div className='table-wrap'><div className='empty'>{t('noDataAvailable')}</div></div>
            ) : (
              <div className='table-wrap'>
                <div className='top10-title'>{t('top10Title')}</div>
                <table>
                  <thead><tr>
                    <th>#</th><th>{t('thPod')}</th><th>{t('thNamespace')}</th><th>{t('thContainer')}</th>
                    <th>{t('thCpuUsage')}</th><th>{t('thCpuPct')}</th>
                    <th>{t('thMemUsage')}</th><th>{t('thMemPct')}</th>
                    <th>{t('thStatus')}</th>
                  </tr></thead>
                  <tbody>
                    {topOffenders.length === 0 && <tr><td colSpan={9} className='empty'>{t('noMetricsData')}</td></tr>}
                    {topOffenders.map((c, i) => (
                      <tr key={i} className={typeof c.cpuPct === 'number' && c.cpuPct >= 90 || typeof c.memPct === 'number' && c.memPct >= 90 ? 'alert-row' : ''}>
                        <td className='rank'>{i + 1}</td>
                        <td>{c.podName}</td>
                        <td>{c.namespace}</td>
                        <td>{c.name}</td>
                        <td>{c.cpu_usage    || '-'}</td>
                        <td className={pctClass(c.cpuPct)}>{fmtPct(c.cpuPct)}</td>
                        <td>{c.memory_usage || '-'}</td>
                        <td className={pctClass(c.memPct)}>{fmtPct(c.memPct)}</td>
                        <td><span className={`dot ${dotStatus(c.cpuPct, c.memPct)}`} /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}

        {/* ── HISTÓRICO ────────────────────────────────────────────────── */}
        {activeTab === 'historico' && (
          <>
            <div className='controls'>
              <select value={histNs} onChange={e => setHistNs(e.target.value)}>
                <option value=''>{t('allNamespacesOpt')}</option>
                {namespaces.map(ns => <option key={ns} value={ns}>{ns}</option>)}
              </select>
              <div className='hist-preset-group'>
                {HIST_PRESETS.map(p => (
                  <button key={p.id} className={`hist-preset-btn ${activePreset === p.id ? 'active' : ''}`}
                    onClick={() => applyPreset(p.id)}>{p.label}</button>
                ))}
              </div>
              <div className='date-range-group'>
                <div className='date-field'>
                  <label>{t('dateFrom')}</label>
                  <input type='date' value={histStart.slice(0, 10)} onChange={e => onHistDateChange(setHistStart, e.target.value)} />
                </div>
                <span className='date-range-sep'>—</span>
                <div className='date-field'>
                  <label>{t('dateTo')}</label>
                  <input type='date' value={histEnd.slice(0, 10)} onChange={e => onHistDateChange(setHistEnd, e.target.value)} />
                </div>
              </div>
              <button onClick={fetchHistory} disabled={histLoading}>{histLoading ? t('fetchingBtn') : t('fetchBtn')}</button>
              <button className='btn-secondary' onClick={exportHistoryCSV} disabled={histLoading}>{t('csvExport')}</button>
            </div>
            {histError && <div className='error-box'>{histError}</div>}
            {!histLoading && history.length === 0 && !histError && (
              <div className='table-wrap'>
                <div className='empty'>{t('noHistoryYet')}</div>
              </div>
            )}
            {histSessions.map(session => (
              <div key={session.id} className='hist-session'>
                <div className='hist-session-header'>
                  {t('historyQuery')(fmtTime(session.at))}
                  <span className='hist-count'>{t('containersCount')(session.records.length)}</span>
                </div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr>
                      <th>{t('thNamespace')}</th><th>{t('thPod')}</th><th>{t('thContainer')}</th>
                      <th>{t('thCpuUsageHist')}</th><th>{t('thCpuPctHist')}</th>
                      <th>{t('thMemUsageHist')}</th><th>{t('thMemPctHist')}</th>
                      <th>{t('thCpuLim')}</th><th>{t('thMemLim')}</th>
                      <th>{t('thStatus')}</th>
                    </tr></thead>
                    <tbody>
                      {session.records.map((rec, i) => {
                        const key    = `${rec.namespace}/${rec.pod}:${rec.container}`
                        const trend  = trendMap[`${session.id}:${key}`] || {}
                        const cpuPct = calcPct(rec.cpu_usage,    rec.cpu_limit, parseCPU)
                        const memPct = calcPct(rec.memory_usage, rec.mem_limit, parseMem)
                        return (
                          <tr key={i} className={typeof cpuPct === 'number' && cpuPct >= 90 || typeof memPct === 'number' && memPct >= 90 ? 'alert-row' : ''}>
                            <td>{rec.namespace}</td><td>{rec.pod}</td><td>{rec.container}</td>
                            <td>{rec.cpu_usage    || '-'}<TrendIcon trend={trend.cpu} /></td>
                            <td className={pctClass(cpuPct)}>{fmtPct(cpuPct)}</td>
                            <td>{rec.memory_usage || '-'}<TrendIcon trend={trend.mem} /></td>
                            <td className={pctClass(memPct)}>{fmtPct(memPct)}</td>
                            <td>{rec.cpu_limit || '-'}</td><td>{rec.mem_limit || '-'}</td>
                            <td><span className={`dot ${dotStatus(cpuPct, memPct)}`} /></td>
                          </tr>
                        )
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
          </>
        )}

        {/* ── ÓRFÃOS ───────────────────────────────────────────────────── */}
        {activeTab === 'orphans' && (
          <>
            <div className='controls'>
              <input
                type='text'
                placeholder={t('orphansExcludePlaceholder')}
                value={orphansExclude}
                onChange={e => setOrphansExclude(e.target.value)}
                style={{ minWidth: 280 }}
              />
              <button onClick={fetchOrphans} disabled={orphansLoad}>{orphansLoad ? t('analyzingOrphans') : t('analyzeOrphans')}</button>
            </div>
            {orphansError && <div className='error-box'>{orphansError}</div>}
            {!orphansLoad && orphans.pvcs.length === 0 && orphans.services.length === 0 && orphans.config_maps.length === 0 && orphans.secrets.length === 0 && orphans.ingresses.length === 0 && orphans.service_accounts.length === 0 && (
              <div className='table-wrap'><div className='empty'>{t('clickToAnalyzeOrphans')}</div></div>
            )}

            {orphans.pvcs.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanPvcs')} <span className='orphan-count'>{orphans.pvcs.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thCapacity')}</th><th>{t('thStorageClass')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.pvcs.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td>{r.capacity || '-'}</td><td>{r.storage_class || '-'}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {orphans.services.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanServices')} <span className='orphan-count'>{orphans.services.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thType')}</th><th>{t('thSelector')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.services.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td>{r.type}</td><td className='val-muted'>{r.selector || '-'}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {orphans.ingresses.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanIngresses')} <span className='orphan-count'>{orphans.ingresses.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thHost')}</th><th>{t('thService')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.ingresses.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td>{r.host || '-'}</td><td className='val-muted'>{r.service || '-'}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {orphans.config_maps.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanConfigMaps')} <span className='orphan-count'>{orphans.config_maps.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.config_maps.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {orphans.secrets.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanSecrets')} <span className='orphan-count'>{orphans.secrets.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thType')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.secrets.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td className='val-muted'>{r.type}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {orphans.service_accounts.length > 0 && (
              <div className='orphan-section'>
                <div className='orphan-section-title'>{t('orphanServiceAccounts')} <span className='orphan-count'>{orphans.service_accounts.length}</span></div>
                <div className='table-wrap'>
                  <table>
                    <thead><tr><th>{t('thNamespace')}</th><th>{t('thNome')}</th><th>{t('thAge')}</th></tr></thead>
                    <tbody>
                      {orphans.service_accounts.map((r, i) => (
                        <tr key={i}><td>{r.namespace}</td><td>{r.name}</td><td className='val-muted'>{r.age}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </>
        )}

        {/* ── LOGS ─────────────────────────────────────────────────────── */}
        {activeTab === 'logs' && (
          <>
            <div className='controls'>
              <select value={logCluster} onChange={e => setLogCluster(e.target.value)}>
                <option value=''>{t('clusterPlaceholderLog')}</option>
                {clusters.map(c => <option key={c} value={c}>{c}</option>)}
              </select>
              <select value={logNamespace} onChange={e => { setLogNamespace(e.target.value); setLogPod(''); setLogPods([]); setLogContent('') }}>
                <option value=''>{t('namespacePlaceholder')}</option>
                {logNsList.map(n => <option key={n} value={n}>{n}</option>)}
              </select>
              <button onClick={fetchLogPods} disabled={!logCluster || !logNamespace}>{t('fetchPods')}</button>
              {logPods.length > 0 && (
                <select value={logPod} onChange={e => setLogPod(e.target.value)}>
                  <option value=''>{t('podPlaceholder')}</option>
                  {logPods.map(p => <option key={p.pod} value={p.pod}>{p.pod}</option>)}
                </select>
              )}
              {logContainers.length > 1 && (
                <select value={logContainer} onChange={e => setLogContainer(e.target.value)}>
                  {logContainers.map(c => <option key={c} value={c}>{c}</option>)}
                </select>
              )}
              <select value={logTail} onChange={e => setLogTail(e.target.value)}>
                <option value='100'>{t('lines100')}</option>
                <option value='200'>{t('lines200')}</option>
                <option value='500'>{t('lines500')}</option>
                <option value='1000'>{t('lines1000')}</option>
              </select>
              <button onClick={fetchLogs} disabled={!logPod || logLoad}>
                {logLoad ? t('loadingLog') : t('viewLog')}
              </button>
              {logContent && (
                <button className='log-download-btn' onClick={() => {
                  const fname = `${logPod}${logContainer ? '_' + logContainer : ''}_${new Date().toISOString().slice(0,19).replace(/:/g,'-')}.txt`
                  const a = document.createElement('a')
                  a.href = URL.createObjectURL(new Blob([logContent], { type: 'text/plain' }))
                  a.download = fname; a.click()
                  URL.revokeObjectURL(a.href)
                }}>
                  {t('downloadLog')}
                </button>
              )}
            </div>
            {logError && <div className='error-box'>{logError}</div>}
            <div className='table-wrap' style={{padding: '0.5rem'}}>
              {!logContent && !logLoad && (
                <div className='empty'>{t('selectNsAndPod')}</div>
              )}
              {logContent && (
                <pre style={{
                  margin: 0, padding: '0.75rem', fontSize: '0.78rem',
                  whiteSpace: 'pre-wrap', wordBreak: 'break-all',
                  maxHeight: '70vh', overflowY: 'auto',
                  background: 'var(--bg)', color: 'var(--text)',
                  borderRadius: '4px', lineHeight: '1.5'
                }}>{logContent}</pre>
              )}
            </div>
          </>
        )}

        {/* ── NODES ────────────────────────────────────────────────────── */}
        {activeTab === 'nodes' && (
          <>
            <div className='controls'>
              <button onClick={fetchNodes} disabled={nodesLoad}>{nodesLoad ? t('queryingNodes') : t('queryNodes')}</button>
            </div>
            {nodesError && <div className='error-box'>{nodesError}</div>}
            {nodes.length > 0 && (
              <div className='metrics'>
                <MetricCard label={t('totalNodes')} value={nodes.length} />
                <MetricCard label='Ready'        value={nodes.filter(n => n.status === 'Ready').length} />
                <MetricCard label='Not Ready'    value={nodes.filter(n => n.status !== 'Ready').length} alert={nodes.some(n => n.status !== 'Ready')} />
              </div>
            )}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>{t('thNodeCol')}</th><th>{t('thNodeStatus')}</th><th>{t('thNodeRole')}</th>
                  <th>{t('thCpuAlloc')}</th><th>{t('thCpuUsageN')}</th><th>{t('thCpuPctN')}</th>
                  <th>{t('thMemAlloc')}</th><th>{t('thMemUsageN')}</th><th>{t('thMemPctN')}</th>
                </tr></thead>
                <tbody>
                  {nodes.length === 0 && <tr><td colSpan={9} className='empty'>{t('noNodesToShow')}</td></tr>}
                  {nodes.map((n, i) => {
                    const cpuAlloc = parseCPU(n.cpu_allocatable), cpuUsed = parseCPU(n.cpu_usage)
                    const memAlloc = parseMem(n.mem_allocatable), memUsed = parseMem(n.mem_usage)
                    const cpuPct = cpuAlloc > 0 && cpuUsed > 0 ? Math.round(cpuUsed / cpuAlloc * 100) : null
                    const memPct = memAlloc > 0 && memUsed > 0 ? Math.round(memUsed / memAlloc * 100) : null
                    const ready  = n.status === 'Ready'
                    return (
                      <tr key={i} className={!ready ? 'alert-row' : ''}>
                        <td>{n.name}</td>
                        <td><span className={`dot ${ready ? 'ok' : 'warn'}`} title={n.status} /> {n.status}</td>
                        <td>{n.role}</td>
                        <td>{n.cpu_allocatable || '-'}</td><td>{n.cpu_usage || '-'}</td>
                        <td className={cpuPct >= 80 ? 'val-alert' : cpuPct >= 60 ? 'val-highlight' : ''}>{cpuPct !== null ? `${cpuPct}%` : '-'}</td>
                        <td>{n.mem_allocatable || '-'}</td><td>{n.mem_usage || '-'}</td>
                        <td className={memPct >= 80 ? 'val-alert' : memPct >= 60 ? 'val-highlight' : ''}>{memPct !== null ? `${memPct}%` : '-'}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── NAMESPACES ───────────────────────────────────────────────── */}
        {activeTab === 'namespaces' && (
          pods.length === 0 ? (
            <div className='table-wrap'><div className='empty'>{t('consultMonitorFirst')}</div></div>
          ) : (
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>{t('thNamespace')}</th>
                  <th>Pods</th><th>{t('containers')}</th>
                  <th>{t('thCpuUsageMilli')}</th><th>{t('thMemUsageMi')}</th>
                  <th>{t('thAlerts90')}</th><th>{t('thCautions85')}</th>
                  <th>{t('thStatus')}</th>
                </tr></thead>
                <tbody>
                  {nsAggregated.length === 0 && <tr><td colSpan={8} className='empty'>{t('noNsData')}</td></tr>}
                  {nsAggregated.map((row, i) => (
                    <tr key={i} className={row.alerts > 0 ? 'alert-row' : ''}>
                      <td>{row.ns}</td>
                      <td>{row.pods}</td>
                      <td>{row.containers}</td>
                      <td>{row.cpuUsage > 0 ? Math.round(row.cpuUsage) + 'm' : '-'}</td>
                      <td>{row.memUsage > 0 ? Math.round(row.memUsage) + ' Mi' : '-'}</td>
                      <td className={row.alerts > 0 ? 'val-alert' : ''}>{row.alerts || '-'}</td>
                      <td className={row.cautions > 0 ? 'val-caution' : ''}>{row.cautions || '-'}</td>
                      <td>
                        <span className={`dot ${row.alerts > 0 ? 'warn' : row.cautions > 0 ? 'caution' : 'ok'}`} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )}

        {/* ── STORAGE ──────────────────────────────────────────────────── */}
        {activeTab === 'storage' && (
          <>
            <div className='controls'>
              <button onClick={fetchStorage} disabled={storageLoad}>{storageLoad ? t('queryingStorage') : t('queryStorage')}</button>
            </div>
            {storageError && <div className='error-box'>{storageError}</div>}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>{t('thNamespace')}</th><th>{t('thPvc')}</th><th>{t('thCapacity')}</th><th>{t('thStatus')}</th>
                  <th>{t('thStorageClass')}</th><th>{t('thVolume')}</th><th>{t('thAccessModes')}</th>
                </tr></thead>
                <tbody>
                  {storage.length === 0 && !storageLoad && (
                    <tr><td colSpan={7} className='empty'>{t('clickToQueryStorage')}</td></tr>
                  )}
                  {storage.map((pvc, i) => (
                    <tr key={i} className={pvc.status === 'Lost' ? 'alert-row' : ''}>
                      <td>{pvc.namespace}</td>
                      <td>{pvc.name}</td>
                      <td>{pvc.capacity || '-'}</td>
                      <td><span className={`dot ${pvc.status === 'Bound' ? 'ok' : pvc.status === 'Pending' ? 'caution' : 'warn'}`} /> {pvc.status}</td>
                      <td>{pvc.storage_class || '-'}</td>
                      <td>{pvc.volume || '-'}</td>
                      <td>{pvc.access_modes || '-'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── CONTAINERS (k8s via Docker host) ────────────────────────── */}
        {activeTab === 'containers' && (
          <>
            <div className='controls'>
              {clusterHosts.length > 1 && (
                <select value={clusterContainerHost} onChange={e => setClusterContainerHost(e.target.value)}>
                  {clusterHosts.map(h => <option key={h.name} value={h.name}>{h.name}</option>)}
                </select>
              )}
              <button onClick={fetchClusterContainers} disabled={clusterContainersLoad}>
                {clusterContainersLoad ? t('queryingContainers') : t('queryContainers')}
              </button>
              {clusterContainers.length > 0 && (
                <select value={clusterFilter} onChange={e => setClusterFilter(e.target.value)}>
                  <option value='all'>{t('allContainersFilter')(clusterContainers.length)}</option>
                  <option value='running'>{t('runningContainers')(clusterContainers.filter(c => c.state === 'running').length)}</option>
                  <option value='exited'>{t('exitedContainers')(clusterContainers.filter(c => c.state !== 'running').length)}</option>
                </select>
              )}
            </div>
            {clusterContainersErr && <div className='error-box'>{clusterContainersErr}</div>}
            {clusterHosts.length === 0 && !clusterContainersLoad && (
              <div className='table-wrap'><div className='empty'>{t('noClusterHostDetected')}</div></div>
            )}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>{t('thNamespace')}</th><th>{t('thPod')}</th><th>{t('thContainer')}</th><th>{t('thImage')}</th>
                  <th>{t('thState')}</th><th>CPU %</th><th>{t('thMemUsageC')}</th><th>{t('thMemLimitC')}</th><th>{t('thMemPctC')}</th>
                </tr></thead>
                <tbody>
                  {clusterContainers.length === 0 && !clusterContainersLoad && clusterHosts.length > 0 && (
                    <tr><td colSpan={9} className='empty'>{t('clickToQueryContainers')}</td></tr>
                  )}
                  {clusterContainers
                    .filter(c => clusterFilter === 'all' || (clusterFilter === 'running' ? c.state === 'running' : c.state !== 'running'))
                    .map((c, i) => {
                      const cpuAlert = c.cpu_pct > 80
                      const memAlert = c.mem_pct > 80
                      const isRunning = c.state === 'running'
                      return (
                        <tr key={i} className={cpuAlert || memAlert ? 'alert-row' : ''}>
                          <td>{c.namespace}</td>
                          <td>{c.pod}</td>
                          <td><strong>{c.container}</strong></td>
                          <td style={{maxWidth:'200px',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}} title={c.image}>{c.image}</td>
                          <td><span className={`dot ${isRunning ? 'ok' : 'warn'}`} /> {c.state}</td>
                          <td className={cpuAlert ? 'alert' : ''}>{isRunning ? `${c.cpu_pct.toFixed(2)}%` : '-'}</td>
                          <td>{isRunning ? formatBytes(c.mem_usage) : '-'}</td>
                          <td>{formatBytes(c.mem_limit)}</td>
                          <td className={memAlert ? 'alert' : ''}>{isRunning && c.mem_pct > 0 ? `${c.mem_pct.toFixed(1)}%` : '-'}</td>
                        </tr>
                      )
                    })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── DOCKER/PODMAN ────────────────────────────────────────────── */}
        {activeTab === 'docker' && (
          <>
            <div className='controls'>
              <button onClick={fetchDockerContainers} disabled={dockerLoad}>
                {dockerLoad ? t('updatingBtn') : t('updateBtn')}
              </button>
              {dockerContainers.length > 0 && (
                <select value={dockerFilter} onChange={e => setDockerFilter(e.target.value)}>
                  <option value='all'>{t('allDockerFilter')(dockerContainers.length)}</option>
                  <option value='running'>{t('runningDockerFilter')(dockerContainers.filter(c => c.state === 'running').length)}</option>
                </select>
              )}
            </div>
            {dockerError && <div className='error-box'>{dockerError}</div>}
            {externalHosts.length === 0 && !dockerLoad && !dockerError && (
              <div className='table-wrap'><div className='empty'>{t('noExternalHostConfigured')}</div></div>
            )}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>Host</th><th>{t('thNome')}</th><th>{t('thImage')}</th><th>{t('thState')}</th><th>CPU %</th>
                  <th>{t('thMemUsageC')}</th><th>{t('thMemLimitC')}</th><th>{t('thMemPctC')}</th><th>{t('thUptime')}</th>
                </tr></thead>
                <tbody>
                  {dockerContainers.length === 0 && !dockerLoad && externalHosts.length > 0 && (
                    <tr><td colSpan={9} className='empty'>{t('loadingContainers')}</td></tr>
                  )}
                  {dockerContainers
                    .filter(c => dockerFilter === 'all' || c.state === 'running')
                    .map((c, i) => {
                      const cpuAlert = c.cpu_pct > 80
                      const memAlert = c.mem_pct > 80
                      const isRunning = c.state === 'running'
                      return (
                        <tr key={i} className={cpuAlert || memAlert ? 'alert-row' : ''}>
                          <td><span style={{fontSize:'0.85em',opacity:.7}}>{c.source_host}</span></td>
                          <td><strong>{c.name}</strong><br/><span style={{fontSize:'0.8em',opacity:.6}}>{c.id}</span></td>
                          <td style={{maxWidth:'220px',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}} title={c.image}>{c.image}</td>
                          <td><span className={`dot ${isRunning ? 'ok' : 'warn'}`} /> {c.state}</td>
                          <td className={cpuAlert ? 'alert' : ''}>{isRunning ? `${c.cpu_pct.toFixed(2)}%` : '-'}</td>
                          <td>{isRunning ? formatBytes(c.mem_usage) : '-'}</td>
                          <td>{formatBytes(c.mem_limit)}</td>
                          <td className={memAlert ? 'alert' : ''}>{isRunning && c.mem_pct > 0 ? `${c.mem_pct.toFixed(1)}%` : '-'}</td>
                          <td>{c.status}</td>
                        </tr>
                      )
                    })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── DEPLOYMENTS ──────────────────────────────────────────────── */}
        {activeTab === 'deployments' && (
          <>
            <div className='controls'>
              <button onClick={fetchDeployments} disabled={deploymentsLoad}>
                {deploymentsLoad ? t('consultingBtn') : t('updateBtn')}
              </button>
            </div>
            {deploymentsError && <div className='error-box'>{deploymentsError}</div>}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>Deployment</th><th>{t('thNamespace')}</th>
                  <th>{t('thDesired')}</th><th>{t('thReady')}</th><th>{t('thAvailable')}</th><th>{t('thUnavailable')}</th><th>{t('thUpdated')}</th>
                  <th>{t('thImages')}</th><th>{t('thStrategy')}</th><th>{t('thRevision')}</th><th>{t('thAge')}</th><th>{t('thStatus')}</th>
                </tr></thead>
                <tbody>
                  {deployments.length === 0 && !deploymentsLoad && (
                    <tr><td colSpan={12} className='empty'>
                      {deploymentsError ? '' : t('noDeploymentFound')}
                    </td></tr>
                  )}
                  {deployments.map((d, i) => {
                    const degraded    = d.unavailable > 0
                    const progressing = !degraded && d.ready < d.desired
                    const badgeCls    = degraded ? 'badge warn' : progressing ? 'badge caution' : 'badge ok'
                    const badgeTxt    = degraded ? 'Degraded' : progressing ? 'Progressing' : 'Healthy'
                    return (
                      <tr key={i} className={degraded ? 'alert-row' : ''}>
                        <td><strong>{d.name}</strong></td>
                        <td>{d.namespace}</td>
                        <td>{d.desired}</td>
                        <td>{d.ready}</td>
                        <td>{d.available}</td>
                        <td className={degraded ? 'val-alert' : ''}>{d.unavailable > 0 ? d.unavailable : '-'}</td>
                        <td>{d.up_to_date}</td>
                        <td style={{maxWidth:'260px',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}} title={d.images?.join('\n')}>
                          {d.images?.map((img, j) => <div key={j} style={{fontSize:'11px',opacity:.85}}>{img}</div>)}
                        </td>
                        <td>{d.strategy}</td>
                        <td>{d.revision || '-'}</td>
                        <td>{d.age}</td>
                        <td><span className={badgeCls}>{badgeTxt}</span></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── HELM ─────────────────────────────────────────────────────── */}
        {activeTab === 'helm' && (
          <>
            <div className='controls'>
              <button onClick={fetchHelmReleases} disabled={helmLoad}>
                {helmLoad ? t('consultingBtn') : t('updateBtn')}
              </button>
            </div>
            {helmError && <div className='error-box'>{helmError}</div>}
            <div className='table-wrap'>
              <table>
                <thead><tr>
                  <th>Release</th><th>{t('thChart')}</th><th>{t('thChartVersion')}</th><th>{t('thAppVersion')}</th>
                  <th>{t('thNamespace')}</th><th>{t('thRevision')}</th><th>{t('thLastUpdated')}</th><th>{t('thStatus')}</th>
                </tr></thead>
                <tbody>
                  {helmReleases.length === 0 && !helmLoad && (
                    <tr><td colSpan={8} className='empty'>
                      {helmError ? '' : t('noHelmRelease')}
                    </td></tr>
                  )}
                  {helmReleases.map((r, i) => {
                    const isOk      = r.status === 'deployed'
                    const isFailed  = r.status === 'failed'
                    const badgeCls  = isOk ? 'badge ok' : isFailed ? 'badge warn' : 'badge caution'
                    const dt = r.last_deployed ? new Date(r.last_deployed).toLocaleString('pt-BR') : '-'
                    return (
                      <tr key={i}>
                        <td><strong>{r.name}</strong></td>
                        <td>{r.chart || '-'}</td>
                        <td>{r.chart_version || '-'}</td>
                        <td>{r.app_version || '-'}</td>
                        <td>{r.namespace}</td>
                        <td>{r.revision}</td>
                        <td>{dt}</td>
                        <td><span className={badgeCls}>{r.status}</span></td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {/* ── ADMIN ────────────────────────────────────────────────────── */}
        {activeTab === 'admin' && (
          <div className='admin-wrap'>
            {/* Modal de confirmação de exclusão */}
            {delClusterModal && (
              <div className='modal-overlay' onClick={() => { setDelClusterModal(null); setDelClusterPwd(''); setDelClusterErr('') }}>
                <div className='modal-box' onClick={e => e.stopPropagation()} style={{ maxWidth: 400 }}>
                  <div className='modal-header'>
                    <span>{t('removeCluster')}</span>
                    <button className='modal-close' onClick={() => { setDelClusterModal(null); setDelClusterPwd(''); setDelClusterErr('') }}>✕</button>
                  </div>
                  <div className='modal-body' style={{ padding: '1rem' }}>
                    <p style={{ color: 'var(--text-2)', marginBottom: '1rem', fontSize: 14 }}>
                      {t('removeClusterConfirm')(delClusterModal)} <strong style={{ color: 'var(--danger)' }}>{delClusterModal}</strong>? {t('removeClusterNote')}
                    </p>
                    <label style={{ fontSize: 12, color: 'var(--text-3)', display: 'block', marginBottom: 6 }}>
                      {t('adminPasswordLabel')}
                    </label>
                    <input
                      type='password'
                      className='dash-name-input'
                      placeholder={t('passwordPlaceholder')}
                      value={delClusterPwd}
                      autoFocus
                      onChange={e => { setDelClusterPwd(e.target.value); setDelClusterErr('') }}
                      onKeyDown={e => e.key === 'Enter' && delClusterPwd && adminDeleteCluster()}
                    />
                    {delClusterErr && <div style={{ color: 'var(--danger)', fontSize: 12, marginTop: 6 }}>{delClusterErr}</div>}
                  </div>
                  <div className='modal-footer'>
                    <button onClick={() => { setDelClusterModal(null); setDelClusterPwd(''); setDelClusterErr('') }}>{t('cancel')}</button>
                    <button
                      className='modal-save'
                      style={{ background: 'var(--danger)', borderColor: 'var(--danger)' }}
                      onClick={adminDeleteCluster}
                      disabled={!delClusterPwd || delClusterLoading}>
                      {delClusterLoading ? t('removing') : t('remove')}
                    </button>
                  </div>
                </div>
              </div>
            )}

            <div className='admin-header'>
              <h2>{t('clusterManagement')}</h2>
            </div>
            {adminMsg.text && <div className={`admin-msg ${adminMsg.type}`}>{adminMsg.text}</div>}

            {/* NOC Mode */}
            <div className='admin-step active noc-section'>
              <div className='admin-step-title'>{t('nocTitle')}</div>
              <div className='admin-step-body'>
                <label className='noc-toggle-row'>
                  <input
                    type='checkbox'
                    checked={nocMode}
                    onChange={e => {
                      const v = e.target.checked
                      setNocMode(v)
                      localStorage.setItem('pm_noc', JSON.stringify(v))
                    }}
                  />
                  <span>{t('nocEnable')}</span>
                </label>

                <div className={`noc-options ${nocMode ? '' : 'noc-options--disabled'}`}>
                  <div className='noc-row'>
                    <span className='noc-label'>{t('nocInterval')}</span>
                    <label className='noc-radio'>
                      <input
                        type='radio' name='noc-interval' value='5'
                        checked={nocInterval === 5}
                        disabled={!nocMode}
                        onChange={() => { setNocInterval(5); localStorage.setItem('pm_noc_interval', '5') }}
                      />
                      {t('nocIntervalOpt5')}
                    </label>
                    <label className='noc-radio'>
                      <input
                        type='radio' name='noc-interval' value='10'
                        checked={nocInterval === 10}
                        disabled={!nocMode}
                        onChange={() => { setNocInterval(10); localStorage.setItem('pm_noc_interval', '10') }}
                      />
                      {t('nocIntervalOpt10')}
                    </label>
                  </div>

                  <div className='noc-row'>
                    <span className='noc-label'>{t('nocModulesLabel')}</span>
                    {[
                      { id: 'monitor',    label: t('nocModMonitor') },
                      { id: 'namespaces', label: t('nocModNamespaces') },
                      { id: 'containers', label: t('nocModContainers') },
                    ].map(({ id, label }) => (
                      <label key={id} className='noc-radio'>
                        <input
                          type='checkbox'
                          checked={nocModules.includes(id)}
                          disabled={!nocMode}
                          onChange={e => {
                            const next = e.target.checked
                              ? [...nocModules, id]
                              : nocModules.filter(m => m !== id)
                            if (next.length === 0) return
                            setNocModules(next)
                            localStorage.setItem('pm_noc_modules', JSON.stringify(next))
                          }}
                        />
                        {label}
                      </label>
                    ))}
                  </div>
                </div>
              </div>
            </div>

            {/* Clusters ativos */}
            <div className='admin-step active'>
              <div className='admin-step-title'>{t('activeClusters')}</div>
              <div className='admin-step-body'>
                {clusters.length === 0 ? (
                  <span style={{ color: 'var(--text-3)', fontSize: 13 }}>{t('noClusterRegistered')}</span>
                ) : (
                  <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                      <tr>
                        <th style={{ textAlign: 'left', padding: '4px 8px', fontSize: 11, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{t('thNameCol')}</th>
                        <th style={{ width: 80 }}></th>
                      </tr>
                    </thead>
                    <tbody>
                      {clusters.map(c => (
                        <tr key={c} style={{ borderTop: '1px solid var(--border)' }}>
                          <td style={{ padding: '8px 8px', color: 'var(--text-1)', fontSize: 13, fontFamily: 'monospace' }}>
                            {c}
                            {c === cluster && <span style={{ marginLeft: 8, fontSize: 10, color: 'var(--accent-text)', background: 'var(--accent-bg)', padding: '1px 6px', borderRadius: 4, fontFamily: 'inherit' }}>{t('activeLabel')}</span>}
                          </td>
                          <td style={{ padding: '4px 8px', textAlign: 'right' }}>
                            <button
                              onClick={() => { setDelClusterModal(c); setDelClusterPwd(''); setDelClusterErr('') }}
                              style={{
                                fontSize: 11, padding: '3px 10px', borderRadius: 4, cursor: 'pointer',
                                background: 'var(--danger-bg)', border: '1px solid var(--danger-border)',
                                color: 'var(--danger-text)',
                              }}>
                              {t('removeBtn')}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            </div>

            <div className={`admin-step ${adminStep >= 1 ? 'active' : ''}`}>
              <div className='admin-step-title'>
                <span className='admin-step-num'>1</span>{t('step1Title')}
              </div>
              <div className='admin-step-body'>
                <div className='admin-row'>
                  <label className='admin-file-btn'>
                    {t('selectFile')}
                    <input type='file' accept='.yaml,.yml,.kubeconfig,*' onChange={adminHandleFile} />
                  </label>
                  <span className='admin-or'>{t('orPasteBelow')}</span>
                </div>
                <textarea className='admin-textarea' placeholder={t('kubeconfigPlaceholder')}
                  value={adminKubeconfig} rows={10}
                  onChange={e => { setAdminKubeconfig(e.target.value); setAdminContexts([]); setAdminStep(1) }} />
                <button className='admin-btn' onClick={adminValidate} disabled={adminLoading || !adminKubeconfig.trim()}>
                  {adminLoading && adminStep === 1 ? t('validating') : t('validateKubeconfig')}
                </button>
              </div>
            </div>

            {adminContexts.length > 0 && (
              <div className={`admin-step ${adminStep >= 2 ? 'active' : ''}`}>
                <div className='admin-step-title'>
                  <span className='admin-step-num'>2</span>{t('step2Title')}
                  <span className='admin-step-hint'>{t('step2Hint')}</span>
                </div>
                <div className='admin-step-body'>
                  <div className='admin-fields'>
                    <div className='admin-field'>
                      <label>{t('contextLabel')}</label>
                      <select value={adminSelCtx} onChange={e => setAdminSelCtx(e.target.value)}>
                        {adminContexts.map(c => <option key={c} value={c}>{c}</option>)}
                      </select>
                    </div>
                    <div className='admin-field'>
                      <label>{t('saNsLabel')}</label>
                      <input type='text' value={adminNs} onChange={e => setAdminNs(e.target.value)} placeholder='default' />
                    </div>
                  </div>
                  <button className='admin-btn' onClick={adminCreateSA} disabled={adminLoading || !adminSelCtx}>
                    {adminLoading ? t('creatingServiceAccount') : t('createSABtn')}
                  </button>
                  {adminSAKubeconfig && <div className='admin-sa-ok'>{t('saCreated')}</div>}
                  <div className='admin-skip'>
                    {t('skipStepText')} <button className='admin-link' onClick={() => setAdminStep(3)}>{t('skipStep')}</button> {t('skipStepSuffix')}
                  </div>
                </div>
              </div>
            )}

            {adminContexts.length > 0 && (
              <div className={`admin-step ${adminStep >= 3 ? 'active' : ''}`}>
                <div className='admin-step-title'>
                  <span className='admin-step-num'>3</span>Aplicar
                  <span className='admin-step-hint'>{adminSAKubeconfig ? 'Usando kubeconfig com token SA' : 'Usando kubeconfig original'}</span>
                </div>
                <div className='admin-step-body'>
                  <p className='admin-apply-desc'>
                    Mescla o novo cluster ao kubeconfig existente, atualiza o Secret
                    <code>pod-monitor-kubeconfig</code> e reinicia o backend.
                  </p>
                  <button className='admin-btn admin-btn-apply' onClick={adminApply} disabled={adminLoading}>
                    {adminLoading ? 'Aplicando...' : 'Aplicar e reiniciar backend'}
                  </button>
                </div>
              </div>
            )}

            <div className='admin-step active' style={{marginTop:'2rem'}}>
              <div className='admin-step-title'>{t('dockerHostsTitle')}</div>
              <div className='admin-step-body'>
                {dockerHostMsg.text && (
                  <div className={`admin-msg ${dockerHostMsg.type}`} style={{marginBottom:'1rem'}}>{dockerHostMsg.text}</div>
                )}
                {dockerHosts.length > 0 && (
                  <table style={{width:'100%',marginBottom:'1rem',borderCollapse:'collapse'}}>
                    <thead><tr>
                      <th style={{textAlign:'left',padding:'4px 8px'}}>{t('thNameCol')}</th>
                      <th style={{textAlign:'left',padding:'4px 8px'}}>{t('thAddrCol')}</th>
                      <th style={{textAlign:'left',padding:'4px 8px'}}>{t('thTypeCol')}</th>
                      <th></th>
                    </tr></thead>
                    <tbody>
                      {dockerHosts.map(h => (
                        <tr key={h.name} style={{borderTop:'1px solid var(--border)'}}>
                          <td style={{padding:'6px 8px'}}><strong>{h.name}</strong></td>
                          <td style={{padding:'6px 8px',fontSize:'0.85em',opacity:.8}}>{h.address || '—'}</td>
                          <td style={{padding:'6px 8px'}}>
                            <span className={`badge ${h.cluster ? 'ok' : 'caution'}`}>{h.cluster ? 'Kubernetes' : 'Docker/Podman'}</span>
                          </td>
                          <td style={{padding:'6px 8px'}}>
                            <button className='admin-link' style={{color:'var(--alert)'}} onClick={() => removeDockerHost(h.name)} disabled={dockerHostLoading}>
                              {t('removeHost')}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
                <div className='admin-fields'>
                  <div className='admin-field'>
                    <label>{t('hostNameLabel')}</label>
                    <input type='text' placeholder={t('hostNamePlaceholder')} value={dockerHostName} onChange={e => setDockerHostName(e.target.value)} />
                  </div>
                  <div className='admin-field' style={{flex:2}}>
                    <label>{t('hostAddrLabel')}</label>
                    <input type='text' placeholder={t('hostAddrPlaceholder')} value={dockerHostAddr} onChange={e => setDockerHostAddr(e.target.value)} />
                  </div>
                </div>
                <button className='admin-btn' onClick={addDockerHost}
                  disabled={dockerHostLoading || !dockerHostName.trim() || !dockerHostAddr.trim()}>
                  {dockerHostLoading ? t('processing') : t('addHost')}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ── DASHBOARDS ───────────────────────────────────────────────── */}
        {activeTab === 'dashboards' && (
          <DashboardPage cluster={cluster} user={user} lang={lang} />
        )}

        {/* ── ANÁLISE ──────────────────────────────────────────────────── */}
        {activeTab === 'analysis' && (
          <AnalysisTab clusters={clusters} />
        )}

        {/* ── TOPOLOGIA ────────────────────────────────────────────────── */}
        {activeTab === 'topology' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div className='controls' style={{ flexWrap: 'wrap', gap: 8 }}>
              <h2 style={{ margin: 0, fontSize: 16, color: 'var(--text-1)' }}>{t('topoTitle')}</h2>
              <span style={{ color: 'var(--text-3)', fontSize: 12 }}>{t('topoSubtitle')}</span>
              <div style={{ marginLeft: 'auto', display: 'flex', gap: 8, alignItems: 'center' }}>
                <input
                  type='text'
                  value={topoNsFilter}
                  onChange={e => { setTopoNsFilter(e.target.value); setTopoNs('') }}
                  placeholder={t('topoNsFilterPlaceholder')}
                  style={{ width: 140, fontSize: 12 }}
                  title={t('topoNsFilterTitle')}
                />
                <select value={topoNs} onChange={e => setTopoNs(e.target.value)} style={{ maxWidth: 200 }}>
                  <option value=''>{t('topoAllNs')}</option>
                  {topoNsFiltered.map(ns => <option key={ns} value={ns}>{ns}</option>)}
                </select>
                <button onClick={fetchTopology} disabled={topoLoad || !cluster}>
                  {topoLoad ? t('topoLoadingBtn') : t('topoLoadBtn')}
                </button>
                <button
                  onClick={() => setTopoAutoRefresh(v => !v)}
                  title={t('topoAutoRefreshTitle')}
                  style={{
                    fontSize: 12, padding: '3px 8px',
                    background: topoAutoRefresh ? 'var(--accent)' : 'var(--surface-2)',
                    color: topoAutoRefresh ? '#fff' : 'var(--text-2)',
                    border: '1px solid var(--border)', borderRadius: 4, cursor: 'pointer',
                    display: 'flex', alignItems: 'center', gap: 4
                  }}
                >
                  <span style={{
                    width: 7, height: 7, borderRadius: '50%',
                    background: topoSseConn && topoAutoRefresh ? '#4ade80' : '#94a3b8',
                    display: 'inline-block',
                    boxShadow: topoSseConn && topoAutoRefresh ? '0 0 4px #4ade80' : 'none'
                  }} />
                  {t('topoLive')}
                </button>
                {topoData && (
                  <span style={{ color: 'var(--text-3)', fontSize: 12 }}>
                    {t('topoNodes')(topoData.nodes?.length || 0)} · {t('topoEdges')(topoData.edges?.length || 0)}
                  </span>
                )}
              </div>
            </div>
            {topoError && <div className='error-box' style={{ flexShrink: 0 }}>{topoError}</div>}
            {!topoData && !topoLoad && !topoError && (
              <div className='empty' style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)', flexShrink: 0 }}>
                {t('topoClickToLoad')}
              </div>
            )}
            {topoData && topoData.nodes?.length === 0 && (
              <div className='empty' style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)', flexShrink: 0 }}>
                {t('topoNoData')}
              </div>
            )}
            {topoData && topoData.nodes?.length > 0 && (
              <TopoGraph nodes={topoData.nodes} edges={topoData.edges} lang={lang} />
            )}
          </div>
        )}

        {/* ── QUOTAS ───────────────────────────────────────────────────── */}
        {activeTab === 'quotas' && (
          <>
            <div className='controls' style={{ flexWrap: 'wrap', gap: 8 }}>
              <h2 style={{ margin: 0, fontSize: 16, color: 'var(--text-1)' }}>{t('quotasTitle')}</h2>
              <span style={{ color: 'var(--text-3)', fontSize: 12 }}>{t('quotasSubtitle')}</span>
              <button onClick={fetchQuotas} disabled={quotasLoad || !cluster} style={{ marginLeft: 'auto' }}>
                {quotasLoad ? t('quotasLoadingBtn') : t('quotasLoadBtn')}
              </button>
            </div>
            {quotasError && <div className='error-box'>{quotasError}</div>}
            {!quotasLoad && quotasData.length === 0 && !quotasError && (
              <div className='empty' style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-3)' }}>
                {t('quotasClickToLoad')}
              </div>
            )}
            {quotasData.map(ns => (
              <div key={ns.namespace} className='admin-step active' style={{ marginBottom: '1rem' }}>
                <div className='admin-step-title' style={{ fontSize: 14 }}>
                  <strong>{ns.namespace}</strong>
                </div>
                <div className='admin-step-body'>
                  {ns.quotas.length === 0 && ns.limit_ranges.length === 0 ? (
                    <span style={{ color: 'var(--text-3)', fontSize: 12 }}>{t('quotasNoData')}</span>
                  ) : (
                    <>
                      {ns.quotas.map(q => (
                        <div key={q.name} style={{ marginBottom: '1rem' }}>
                          <div style={{ fontSize: 12, color: 'var(--text-3)', marginBottom: 4 }}>{q.name}</div>
                          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
                            <thead><tr>
                              <th style={{ textAlign: 'left', padding: '2px 8px' }}>{t('thNameCol')}</th>
                              <th style={{ textAlign: 'right', padding: '2px 8px' }}>{t('quotasUsed')}</th>
                              <th style={{ textAlign: 'right', padding: '2px 8px' }}>{t('quotasHard')}</th>
                              <th style={{ textAlign: 'right', padding: '2px 8px' }}>{t('quotasPct')}</th>
                            </tr></thead>
                            <tbody>
                              {Object.entries(q.resources).map(([res, val]) => {
                                const usedNum  = parseFloat(val.used)  || 0
                                const hardNum  = parseFloat(val.hard)  || 0
                                const pct = hardNum > 0 ? Math.round(usedNum / hardNum * 100) : null
                                return (
                                  <tr key={res} style={{ borderTop: '1px solid var(--border)' }}>
                                    <td style={{ padding: '3px 8px' }}><code style={{ fontSize: 11 }}>{res}</code></td>
                                    <td style={{ textAlign: 'right', padding: '3px 8px' }}>{val.used || '0'}</td>
                                    <td style={{ textAlign: 'right', padding: '3px 8px' }}>{val.hard}</td>
                                    <td style={{ textAlign: 'right', padding: '3px 8px' }}>
                                      {pct !== null ? (
                                        <span className={pct >= 90 ? 'val-alert' : pct >= 75 ? 'val-caution' : ''}>
                                          {pct}%
                                        </span>
                                      ) : '—'}
                                    </td>
                                  </tr>
                                )
                              })}
                            </tbody>
                          </table>
                        </div>
                      ))}
                      {ns.limit_ranges.length > 0 && (
                        <div style={{ marginTop: 8 }}>
                          <div style={{ fontSize: 11, color: 'var(--text-3)', marginBottom: 4 }}>LimitRanges</div>
                          {ns.limit_ranges.map((lr, i) => (
                            <div key={i} style={{ fontSize: 11, color: 'var(--text-2)', padding: '2px 0' }}>
                              <strong>{lr.name}</strong> [{lr.type}]
                              {lr.default && Object.keys(lr.default).length > 0 && (
                                <span style={{ marginLeft: 8 }}>
                                  {t('quotasDefault')}: {Object.entries(lr.default).map(([k, v]) => `${k}=${v}`).join(', ')}
                                </span>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </>
                  )}
                </div>
              </div>
            ))}
          </>
        )}

        {/* ── ADMIN (extras: audit, webhooks, thresholds) ───────────────── */}
        {activeTab === 'admin' && isAdmin && (
          <div style={{ marginTop: '2rem' }}>

            {/* Alert Thresholds */}
            <div className='admin-step active' style={{ marginBottom: '1.5rem' }}>
              <div className='admin-step-title'>{t('thresholdsTitle')}</div>
              <div className='admin-step-body'>
                <p style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: '1rem' }}>{t('thresholdsSubtitle')}</p>
                {thrMsg.text && <div className={`admin-msg ${thrMsg.type}`} style={{ marginBottom: '1rem' }}>{thrMsg.text}</div>}
                {thresholds.length === 0 ? (
                  <div style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: '1rem' }}>{t('thresholdNoData')}</div>
                ) : (
                  <table style={{ width: '100%', borderCollapse: 'collapse', marginBottom: '1rem', fontSize: 12 }}>
                    <thead><tr>
                      <th style={{ textAlign: 'left', padding: '4px 8px' }}>{t('thresholdCluster')}</th>
                      <th style={{ textAlign: 'left', padding: '4px 8px' }}>{t('thresholdNamespace')}</th>
                      <th style={{ textAlign: 'center', padding: '4px 8px' }}>{t('thresholdWarn')}</th>
                      <th style={{ textAlign: 'center', padding: '4px 8px' }}>{t('thresholdCrit')}</th>
                      <th></th>
                    </tr></thead>
                    <tbody>
                      {thresholds.map(th => (
                        <tr key={th.id} style={{ borderTop: '1px solid var(--border)' }}>
                          <td style={{ padding: '4px 8px', fontFamily: 'monospace', fontSize: 11 }}>
                            {th.cluster || t('thresholdGlobal')}
                          </td>
                          <td style={{ padding: '4px 8px', fontFamily: 'monospace', fontSize: 11 }}>
                            {th.namespace || t('thresholdClusterOnly')}
                          </td>
                          <td style={{ padding: '4px 8px', textAlign: 'center' }}><span className='val-caution'>{th.warn_pct}%</span></td>
                          <td style={{ padding: '4px 8px', textAlign: 'center' }}><span className='val-alert'>{th.crit_pct}%</span></td>
                          <td style={{ padding: '4px 8px', textAlign: 'right' }}>
                            <button className='admin-link' style={{ color: 'var(--alert)' }} onClick={() => deleteThreshold(th.id)}>
                              {t('thresholdDelete')}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
                <div className='admin-fields'>
                  <div className='admin-field'>
                    <label>{t('thresholdCluster')}</label>
                    <input type='text' placeholder='(global)' value={thrForm.cluster}
                      onChange={e => setThrForm(f => ({ ...f, cluster: e.target.value }))} />
                  </div>
                  <div className='admin-field'>
                    <label>{t('thresholdNamespace')}</label>
                    <input type='text' placeholder='(todos)' value={thrForm.namespace}
                      onChange={e => setThrForm(f => ({ ...f, namespace: e.target.value }))} />
                  </div>
                  <div className='admin-field' style={{ width: 100 }}>
                    <label>{t('thresholdWarn')}</label>
                    <input type='number' min={1} max={99} value={thrForm.warn_pct}
                      onChange={e => setThrForm(f => ({ ...f, warn_pct: +e.target.value }))} />
                  </div>
                  <div className='admin-field' style={{ width: 100 }}>
                    <label>{t('thresholdCrit')}</label>
                    <input type='number' min={1} max={100} value={thrForm.crit_pct}
                      onChange={e => setThrForm(f => ({ ...f, crit_pct: +e.target.value }))} />
                  </div>
                </div>
                <button className='admin-btn' onClick={saveThreshold} disabled={thrLoad}>
                  {thrLoad ? t('thresholdSaving') : t('thresholdSave')}
                </button>
              </div>
            </div>

            {/* Webhooks */}
            <div className='admin-step active' style={{ marginBottom: '1.5rem' }}>
              <div className='admin-step-title'>{t('webhooksTitle')}</div>
              <div className='admin-step-body'>
                <p style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: '1rem' }}>{t('webhooksSubtitle')}</p>
                {whMsg.text && <div className={`admin-msg ${whMsg.type}`} style={{ marginBottom: '1rem' }}>{whMsg.text}</div>}
                {webhooks.length === 0 ? (
                  <div style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: '1rem' }}>{t('webhookNoData')}</div>
                ) : (
                  <table style={{ width: '100%', borderCollapse: 'collapse', marginBottom: '1rem', fontSize: 12 }}>
                    <thead><tr>
                      <th style={{ textAlign: 'left', padding: '4px 8px' }}>{t('webhookName')}</th>
                      <th style={{ textAlign: 'left', padding: '4px 8px' }}>{t('webhookUrl')}</th>
                      <th style={{ textAlign: 'left', padding: '4px 8px' }}>{t('webhookEvents')}</th>
                      <th style={{ textAlign: 'center', padding: '4px 8px' }}>{t('webhookEnabled')}</th>
                      <th></th>
                    </tr></thead>
                    <tbody>
                      {webhooks.map(wh => (
                        <tr key={wh.id} style={{ borderTop: '1px solid var(--border)' }}>
                          <td style={{ padding: '4px 8px' }}><strong>{wh.name}</strong></td>
                          <td style={{ padding: '4px 8px', fontFamily: 'monospace', fontSize: 10 }}>{wh.url}</td>
                          <td style={{ padding: '4px 8px' }}><span className='badge caution'>{wh.events}</span></td>
                          <td style={{ padding: '4px 8px', textAlign: 'center' }}>
                            {wh.enabled ? <span className='badge ok'>✓</span> : <span className='badge'>✗</span>}
                          </td>
                          <td style={{ padding: '4px 8px', textAlign: 'right', display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                            <button className='admin-link' style={{ color: 'var(--accent)' }} onClick={() => testWebhook(wh.url)}>
                              {t('webhookTest') || 'Testar'}
                            </button>
                            <button className='admin-link' style={{ color: 'var(--alert)' }} onClick={() => deleteWebhook(wh.id)}>
                              {t('webhookDelete')}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
                <div className='admin-fields'>
                  <div className='admin-field'>
                    <label>{t('webhookName')}</label>
                    <input type='text' placeholder={t('webhookNamePlaceholder')} value={whForm.name}
                      onChange={e => setWhForm(f => ({ ...f, name: e.target.value }))} />
                  </div>
                  <div className='admin-field' style={{ flex: 2 }}>
                    <label>{t('webhookUrl')}</label>
                    <input type='url' placeholder={t('webhookUrlPlaceholder')} value={whForm.url}
                      onChange={e => setWhForm(f => ({ ...f, url: e.target.value }))} />
                  </div>
                  <div className='admin-field' style={{ width: 150 }}>
                    <label>{t('webhookEvents')}</label>
                    <select value={whForm.events} onChange={e => setWhForm(f => ({ ...f, events: e.target.value }))}>
                      <option value='critical'>{t('webhookEventCritical')}</option>
                      <option value='warning'>{t('webhookEventWarning')}</option>
                      <option value='critical,warning'>{t('webhookEventBoth')}</option>
                    </select>
                  </div>
                  <div className='admin-field' style={{ width: 80 }}>
                    <label>{t('webhookEnabled')}</label>
                    <input type='checkbox' checked={whForm.enabled}
                      onChange={e => setWhForm(f => ({ ...f, enabled: e.target.checked }))} />
                  </div>
                </div>
                <button className='admin-btn' onClick={saveWebhook} disabled={whLoad || !whForm.url.trim()}>
                  {whLoad ? t('webhookSaving') : t('webhookSave')}
                </button>
              </div>
            </div>

            {/* Audit Log */}
            <div className='admin-step active'>
              <div className='admin-step-title'>{t('auditTitle')}</div>
              <div className='admin-step-body'>
                <p style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: '1rem' }}>{t('auditSubtitle')}</p>
                {auditError && <div className='admin-msg error'>{auditError}</div>}
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: '1rem' }}>
                  <span style={{ fontSize: 12, color: 'var(--text-3)' }}>{t('auditLimit')}</span>
                  <select value={auditLimit} onChange={e => setAuditLimit(+e.target.value)}
                    style={{ width: 80, fontSize: 12 }}>
                    {[50, 100, 200, 500].map(n => <option key={n} value={n}>{n}</option>)}
                  </select>
                  <span style={{ fontSize: 12, color: 'var(--text-3)' }}>{t('auditEntries')}</span>
                  <button className='admin-btn' onClick={fetchAudit} disabled={auditLoad} style={{ marginLeft: 8 }}>
                    {auditLoad ? t('auditLoadingBtn') : t('auditLoadBtn')}
                  </button>
                </div>
                {auditData.length === 0 && !auditLoad ? (
                  <div style={{ color: 'var(--text-3)', fontSize: 12 }}>{t('auditNoData')}</div>
                ) : (
                  <div style={{ overflowX: 'auto', maxHeight: 400 }}>
                    <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 11 }}>
                      <thead><tr>
                        <th style={{ textAlign: 'left', padding: '4px 8px', position: 'sticky', top: 0, background: 'var(--bg-2)' }}>{t('auditTimestamp')}</th>
                        <th style={{ textAlign: 'left', padding: '4px 8px', position: 'sticky', top: 0, background: 'var(--bg-2)' }}>{t('auditUser')}</th>
                        <th style={{ textAlign: 'left', padding: '4px 8px', position: 'sticky', top: 0, background: 'var(--bg-2)' }}>{t('auditAction')}</th>
                        <th style={{ textAlign: 'left', padding: '4px 8px', position: 'sticky', top: 0, background: 'var(--bg-2)' }}>{t('auditDetail')}</th>
                        <th style={{ textAlign: 'left', padding: '4px 8px', position: 'sticky', top: 0, background: 'var(--bg-2)' }}>{t('auditIP')}</th>
                      </tr></thead>
                      <tbody>
                        {auditData.map(e => (
                          <tr key={e.id} style={{ borderTop: '1px solid var(--border)' }}>
                            <td style={{ padding: '3px 8px', color: 'var(--text-3)', whiteSpace: 'nowrap' }}>{fmtTime(e.timestamp)}</td>
                            <td style={{ padding: '3px 8px', fontWeight: 600 }}>{e.username}</td>
                            <td style={{ padding: '3px 8px' }}><code style={{ fontSize: 10 }}>{e.action}</code></td>
                            <td style={{ padding: '3px 8px', color: 'var(--text-2)', maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{e.detail}</td>
                            <td style={{ padding: '3px 8px', color: 'var(--text-3)', fontFamily: 'monospace', fontSize: 10 }}>{e.ip}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>

          </div>
        )}

      </main>
      </div>
    </div>
  )
}
