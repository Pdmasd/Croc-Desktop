import { useEffect, useState } from 'react'
import { Activity, ArrowDownToLine, ArrowUpFromLine, Check, ChevronRight, CircleHelp, Clock3, Copy, Folder, HardDrive, LayoutDashboard, LockKeyhole, MoreHorizontal, Plus, Settings2, ShieldCheck, X } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { Events, Clipboard } from '@wailsio/runtime'
import * as DesktopService from '../bindings/github.com/schollz/croc/v11/desktop/desktopservice'

type View = 'Dashboard' | 'Send Files' | 'Receive Files' | 'Active Transfers' | 'Settings'
const nav: { name: View; icon: typeof LayoutDashboard }[] = [
  { name: 'Dashboard', icon: LayoutDashboard }, { name: 'Send Files', icon: ArrowUpFromLine },
  { name: 'Receive Files', icon: ArrowDownToLine }, { name: 'Active Transfers', icon: Activity }, { name: 'Settings', icon: Settings2 },
]

type TransferSnapshot = { id: string; direction: string; state: string; transferred: number; total: number; bytesPerSecond: number; error?: string }
const terminal = (state?: string) => state === 'Completed' || state === 'Failed' || state === 'Cancelled'
const displayName = (path: string) => path.split(/[\\/]/).filter(Boolean).at(-1) || path
const formatBytes = (bytes: number) => bytes < 1024 ? `${bytes} B` : bytes < 1024 ** 2 ? `${(bytes / 1024).toFixed(1)} KB` : `${(bytes / 1024 ** 2).toFixed(1)} MB`

export default function App() {
  const [view, setView] = useState<View>('Dashboard')
  const [files, setFiles] = useState<string[]>([])
  const [code, setCode] = useState('')
  const [copied, setCopied] = useState(false)
  const [secret, setSecret] = useState('')
  const [transfer, setTransfer] = useState<TransferSnapshot | null>(null)
  const [receiveFolder, setReceiveFolder] = useState('')
  const [relayAddress, setRelayAddress] = useState('croc.schollz.com:9009')
  const [relayPassword, setRelayPassword] = useState('pass123')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const stopTransfer = Events.On('transfer-update', (event) => {
      const next = event.data as TransferSnapshot
      setTransfer(next)
      if (terminal(next.state)) setCode('')
      if (next.state === 'Failed') setNotice(next.error || 'Transfer failed.')
      if (next.state === 'Completed') setNotice('Transfer completed successfully.')
      if (next.state === 'Cancelled') setNotice('Transfer cancelled.')
    })
    const stopDrop = Events.On('files-dropped', async (event) => {
      try {
        const paths = event.data as string[]
        const accepted = await DesktopService.SetDroppedFiles(paths)
        setFiles(accepted || [])
        setView('Send Files')
      } catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
    })
    void DesktopService.CurrentTransfer().then((current) => { if (current.id) setTransfer(current as TransferSnapshot) }).catch(() => {})
    void DesktopService.Settings().then((settings) => { setReceiveFolder(settings.receiveFolder); setRelayAddress(settings.relayAddress); setRelayPassword(settings.relayPassword) }).catch(() => {})
    return () => { stopTransfer(); stopDrop() }
  }, [])

  const browse = async () => {
    try { setFiles((await DesktopService.SelectFiles()) || []) }
    catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
  }

  const copyCode = async () => {
    try { await Clipboard.SetText(code); setCopied(true); window.setTimeout(() => setCopied(false), 1500) }
    catch (error) { setNotice(error instanceof Error ? error.message : 'Could not copy the share code.') }
  }

  const startSend = async () => {
    setBusy(true); setNotice('')
    try {
      const result = await DesktopService.StartSend(files)
      const current = await DesktopService.CurrentTransfer() as TransferSnapshot
      setCode(terminal(current.state) ? '' : result.shareCode || '')
      setTransfer(current)
      setView('Active Transfers')
    } catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
    finally { setBusy(false) }
  }

  const startReceive = async () => {
    setBusy(true); setNotice('')
    try {
      await DesktopService.StartReceive(secret)
      setTransfer(await DesktopService.CurrentTransfer() as TransferSnapshot)
      setSecret('')
      setView('Active Transfers')
    } catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
    finally { setBusy(false) }
  }

  const chooseReceiveFolder = async () => {
    try { setReceiveFolder(await DesktopService.SelectReceiveFolder()) }
    catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
  }

  const saveRelay = async () => {
    try { await DesktopService.UpdateRelay(relayAddress, relayPassword); setNotice('Relay settings updated for new transfers.') }
    catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
  }

  const cancelTransfer = async () => {
    try { await DesktopService.CancelTransfer() }
    catch (error) { setNotice(error instanceof Error ? error.message : String(error)) }
  }

  const currentActive = transfer && !terminal(transfer.state)
  const progress = transfer?.total ? Math.min(100, Math.round(transfer.transferred / transfer.total * 100)) : 0

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><div className="brand-mark">c</div><span>Croc<span className="brand-light"> Desktop</span></span><span className="version">BETA</span></div>
      <div className="workspace"><div className="workspace-icon"><HardDrive size={16}/></div><div><b>This device</b><small>Private transfer workspace</small></div><MoreHorizontal size={18} className="muted"/></div>
      <div className="nav-label">WORKSPACE</div>
      <nav>{nav.map(({name, icon: Icon}) => <button key={name} className={`nav-item ${view === name ? 'selected' : ''}`} onClick={() => setView(name)}><Icon size={17}/><span>{name}</span>{name === 'Active Transfers' && <i className="nav-dot"/>}</button>)}</nav>
      <div className="sidebar-bottom"><div className="secure-card"><div className="secure-icon"><ShieldCheck size={16}/></div><div><b>Private by design</b><p>Peer authentication and encrypted transfers powered by Croc.</p></div></div><button className="help-link"><CircleHelp size={16}/> How it works <ChevronRight size={14}/></button><div className="profile"><div className="avatar">C</div><div><b>Croc Desktop</b><small>Local application</small></div><MoreHorizontal size={18} className="muted"/></div></div>
    </aside>

    <main className="main-area">
      <header className="topbar"><div className="crumb">Workspace <ChevronRight size={14}/><b>{view}</b></div><div className="top-actions"><span className="online"><i/> {currentActive ? transfer?.state : 'Ready'}</span><button className="icon-btn" title="Settings" onClick={() => setView('Settings')}><Settings2 size={17}/></button></div></header>
      <div className="content">
        {notice && <div className={`notice ${notice.includes('successfully') ? 'notice-success' : ''}`} role="status"><span>{notice}</span><button onClick={() => setNotice('')} aria-label="Dismiss notification"><X size={14}/></button></div>}
        {view === 'Dashboard' && <>
          <div className="welcome"><div><div className="eyebrow"><LockKeyhole size={13}/> SECURE FILE TRANSFER</div><h1>Your files, <em>on their way.</em></h1><p>Send directly between devices with end-to-end encryption. No accounts, no cloud storage.</p></div><div className="welcome-art"><div className="orbit orbit-one"/><div className="orbit orbit-two"/><div className="art-core"><LockKeyhole size={29}/></div><span className="orbit-node node-a"><ArrowUpFromLine size={14}/></span><span className="orbit-node node-b"><ArrowDownToLine size={14}/></span><span className="art-spark spark-a"/><span className="art-spark spark-b"/></div></div>
          <div className="section-heading"><div><h2>Get started</h2><p>Choose how you want to transfer files.</p></div><span className="shortcut-hint">SECURE Ãƒâ€šÃ‚Â· DIRECT Ãƒâ€šÃ‚Â· SIMPLE</span></div>
          <div className="action-grid"><button className="action-card send-card" onClick={() => setView('Send Files')}><div className="action-icon send-icon"><ArrowUpFromLine size={20}/></div><span className="card-kicker">SEND TO ANOTHER DEVICE</span><strong>Send files</strong><p>Create a secure share code and let another device connect.</p><span className="card-link">Choose files <ChevronRight size={15}/></span><div className="card-decoration send-decoration"><ArrowUpFromLine size={68}/></div></button><button className="action-card receive-card" onClick={() => setView('Receive Files')}><div className="action-icon receive-icon"><ArrowDownToLine size={20}/></div><span className="card-kicker">RECEIVE FROM A DEVICE</span><strong>Receive files</strong><p>Enter a Croc share code to securely receive files.</p><span className="card-link">Enter a code <ChevronRight size={15}/></span><div className="card-decoration receive-decoration"><ArrowDownToLine size={68}/></div></button></div>
          <div className="bottom-grid"><div className="panel how-panel"><div className="panel-title"><div><h3>How Croc keeps it private</h3><p>Your transfer goes directly between devices whenever possible.</p></div><div className="shield-badge"><ShieldCheck size={17}/></div></div><div className="steps"><div><span>01</span><b>Share a code</b><small>A temporary code connects your devices.</small></div><div><span>02</span><b>Verify each other</b><small>PAKE authenticates peers without exposing the secret.</small></div><div><span>03</span><b>Transfer securely</b><small>Your files are encrypted while in transit.</small></div></div></div><div className="panel status-panel"><div className="panel-title"><div><h3>Transfer status</h3><p>Activity on this device</p></div><button className="text-button" onClick={() => setView('Active Transfers')}>View all <ChevronRight size={14}/></button></div>{currentActive ? <div className="empty-state"><div className="empty-icon"><Activity size={19}/></div><div><b>{transfer?.direction === 'send' ? 'Sending files' : 'Receiving files'} Ãƒâ€šÃ‚Â· {transfer?.state}</b><small>{formatBytes(transfer?.transferred || 0)} of {formatBytes(transfer?.total || 0)}</small></div></div> : <div className="empty-state"><div className="empty-icon"><Clock3 size={19}/></div><div><b>{transfer?.state === 'Completed' ? 'Last transfer completed' : 'No active transfers'}</b><small>{transfer?.state === 'Failed' ? transfer.error : 'Your active sessions will appear here.'}</small></div></div>}<div className="status-footer"><span><i/>Croc core ready</span><span>{currentActive ? `${progress}% complete` : 'Waiting for a transfer'}</span></div></div></div>
        </>}
        {view === 'Send Files' && <section className="transfer-view"><div className="eyebrow"><ArrowUpFromLine size={13}/> SEND FILES</div><h1>Share files <em>securely.</em></h1><p className="view-desc">Create a private Croc transfer. Your share code is a secretÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Âonly give it to the recipient.</p><div className="transfer-columns"><div className="panel transfer-panel"><div className="panel-title"><div><h3>Select files</h3><p>Files are sent using CrocÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢s native transfer protocol.</p></div></div><button className="browse-area" onClick={browse}><div className="browse-icon"><Plus size={21}/></div><b>Choose files</b><span>or drag and drop files anywhere in this window</span></button>{files.length > 0 && <div className="file-list">{files.map(f => <div className="file-row" key={f}><Folder size={16}/><span title={f}>{displayName(f)}</span><button onClick={() => setFiles(files.filter(x => x !== f))}><X size={15}/></button></div>)}</div>}<button className="primary-button" disabled={!files.length || busy || !!currentActive} onClick={() => void startSend()}>{busy ? 'Starting transferÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦' : 'Create secure share'} <ArrowUpFromLine size={16}/></button><div className="secure-note"><ShieldCheck size={14}/>Croc keeps the share code in memory for this session only.</div></div><div className="panel share-preview"><div className="panel-title"><div><h3>Share code</h3><p>Give this code to the receiving device.</p></div></div>{code ? <><div className="qr-box"><QRCodeSVG value={`https://getcroc.com/?code=${encodeURIComponent(code)}`} size={138} level="M"/></div><div className="share-code">{code}</div><button className="secondary-button" onClick={() => void copyCode()}>{copied ? <Check size={16}/> : <Copy size={16}/>} {copied ? 'Copied' : 'Copy code'}</button></> : <div className="share-empty"><div className="empty-icon"><LockKeyhole size={19}/></div><b>Your code will appear here</b><small>Choose files to create a live Croc share.</small></div>}<div className="qr-caption">Scan to open CrocÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢s web receiver</div></div></div></section>}
        {view === 'Receive Files' && <section className="transfer-view"><div className="eyebrow"><ArrowDownToLine size={13}/> RECEIVE FILES</div><h1>Receive with <em>Croc.</em></h1><p className="view-desc">Enter the secure code from the sender. Files will be saved to your chosen folder.</p><div className="panel receive-form"><div className="panel-title"><div><h3>Connect to a sender</h3><p>Enter the full Croc share code to authenticate the transfer.</p></div><div className="shield-badge"><LockKeyhole size={16}/></div></div><label htmlFor="code">SHARE CODE</label><input id="code" className="code-input" placeholder="e.g. sunny-river-forest" value={secret} onChange={e => setSecret(e.target.value)} autoComplete="off"/><div className="folder-choice"><div className="folder-badge"><Folder size={17}/></div><div><b>Save location</b><small title={receiveFolder}>{receiveFolder || 'Select a folder for incoming files'}</small></div><button className="outline-button" onClick={() => void chooseReceiveFolder()}><Folder size={14}/> Choose folder</button></div><button className="primary-button" disabled={!secret.trim() || busy || !!currentActive} onClick={() => void startReceive()}>{busy ? 'ConnectingÃƒÂ¢Ã¢â€šÂ¬Ã‚Â¦' : 'Connect and receive'} <ArrowDownToLine size={16}/></button><div className="secure-note"><ShieldCheck size={14}/>Name conflicts are saved with a new filename; existing files are preserved.</div></div></section>}
        {view === 'Active Transfers' && <section className="transfer-view"><div className="eyebrow"><Activity size={13}/> TRANSFER MANAGER</div><h1>Your <em>transfers.</em></h1><p className="view-desc">Monitor the active Croc session and its progress.</p>{transfer ? <div className="panel live-transfer"><div className="live-transfer-head"><div className="action-icon send-icon">{transfer.direction === 'send' ? <ArrowUpFromLine size={19}/> : <ArrowDownToLine size={19}/>}</div><div><h3>{transfer.direction === 'send' ? 'Sending files' : 'Receiving files'}</h3><p>{transfer.state}{transfer.error ? ` Ãƒâ€šÃ‚Â· ${transfer.error}` : ''}</p></div>{currentActive && <button className="cancel-button" onClick={() => void cancelTransfer()}><X size={14}/> Cancel transfer</button>}</div><div className="progress-track"><span style={{width: `${progress}%`}}/></div><div className="progress-meta"><span>{formatBytes(transfer.transferred)} of {formatBytes(transfer.total)}</span><span>{transfer.bytesPerSecond > 0 ? `${formatBytes(transfer.bytesPerSecond)}/s` : 'ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â'}</span><b>{progress}%</b></div>{code && transfer.direction === 'send' && <div className="active-share"><LockKeyhole size={14}/><span>Share code: <b>{code}</b></span><button className="text-button" onClick={() => void copyCode()}>{copied ? 'Copied' : 'Copy'} <Copy size={13}/></button></div>}</div> : <div className="panel no-transfers"><div className="empty-icon"><Activity size={20}/></div><h3>No transfer session yet</h3><p>Start sending or receive with a Croc code. Live progress will appear here.</p><div className="inline-actions"><button className="secondary-button" onClick={() => setView('Send Files')}>Send files</button><button className="secondary-button" onClick={() => setView('Receive Files')}>Receive files</button></div></div>}</section>}
        {view === 'Settings' && <section className="transfer-view"><div className="eyebrow"><Settings2 size={13}/> PREFERENCES</div><h1>App <em>settings.</em></h1><p className="view-desc">Configure where files arrive and which Croc relay new sessions use.</p><div className="panel settings-panel"><div className="setting-row"><div className="folder-badge"><Folder size={17}/></div><div><b>Receive folder</b><small title={receiveFolder}>{receiveFolder || 'Choose where received files are saved.'}</small></div><button className="outline-button" onClick={() => void chooseReceiveFolder()}><Folder size={14}/> Choose folder</button></div><div className="setting-row"><div className="folder-badge"><ShieldCheck size={17}/></div><div><b>File conflict behavior</b><small>Keep both files by choosing a new name. Never overwrite silently.</small></div><span className="setting-value">Keep both</span></div><div className="relay-settings"><div><b>Transfer relay</b><small>Applied to transfers started after saving.</small></div><label>RELAY ADDRESS<input className="code-input" value={relayAddress} onChange={e => setRelayAddress(e.target.value)} placeholder="host:port" autoComplete="off"/></label><label>RELAY PASSWORD<input className="code-input" value={relayPassword} onChange={e => setRelayPassword(e.target.value)} type="password" autoComplete="new-password"/></label><button className="secondary-button" onClick={() => void saveRelay()}>Save relay settings</button></div></div></section>}
      </div>
      <footer className="app-footer"><span><LockKeyhole size={12}/> End-to-end encrypted by Croc</span><span>Transfers run directly between devices when possible <i/></span></footer>
    </main>
  </div>
}
