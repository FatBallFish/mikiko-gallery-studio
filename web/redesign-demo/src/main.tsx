import React, { useState, useEffect } from 'react'
import ReactDOM from 'react-dom/client'
import { RedesignDemo } from './RedesignDemo'
import { AdminDemo } from './admin/AdminDemo'
import './styles.css'

function App() {
  const [view, setView] = useState<'studio' | 'admin'>('studio')

  useEffect(() => {
    const path = window.location.pathname
    if (path.startsWith('/admin')) setView('admin')
    else setView('studio')
    
    // Listen for manual URL changes
    const handlePopState = () => {
      const p = window.location.pathname
      if (p.startsWith('/admin')) setView('admin')
      else setView('studio')
    }
    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  const navigateTo = (to: 'studio' | 'admin') => {
    const newPath = to === 'admin' ? '/admin' : '/'
    window.history.pushState({}, '', newPath)
    setView(to)
  }

  return (
    <div className="relative">
      {/* Floating Toggle for Demo Switching */}
      <div className="fixed bottom-6 right-6 z-[9999] flex gap-2 p-2 bg-black/40 backdrop-blur-xl border border-white/10 rounded-2xl shadow-2xl">
        <button 
          className={`px-4 py-2 rounded-xl text-xs font-bold transition-all ${view === 'studio' ? 'bg-[var(--accent)] text-white' : 'text-white/40 hover:text-white'}`}
          onClick={() => navigateTo('studio')}
        >
          Studio Redesign
        </button>
        <button 
          className={`px-4 py-2 rounded-xl text-xs font-bold transition-all ${view === 'admin' ? 'bg-[var(--accent)] text-white' : 'text-white/40 hover:text-white'}`}
          onClick={() => navigateTo('admin')}
        >
          Admin Redesign
        </button>
      </div>

      {view === 'studio' ? <RedesignDemo /> : <AdminDemo />}
    </div>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
