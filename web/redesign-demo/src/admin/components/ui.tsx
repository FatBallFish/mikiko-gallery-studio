import React from 'react'
import { cn } from '../../../../shared/classnames'
import { rdForm } from '../admin-classes'

export const Modal = ({ isOpen, onClose, title, children, footer, size = 'md' }: any) => {
  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4 bg-black/60 backdrop-blur-md animate-in fade-in duration-200">
      <div 
        className={cn(
          "bg-[#0a0a0a] border border-white/10 rounded-3xl w-full shadow-2xl shadow-black/50 overflow-hidden animate-in zoom-in-95 duration-200 flex flex-col max-h-[90vh]",
          size === 'sm' ? "max-w-sm" : size === 'md' ? "max-w-lg" : size === 'lg' ? "max-w-2xl" : "max-w-4xl"
        )}
      >
        <div className="flex items-center justify-between p-6 border-b border-white/5 shrink-0">
          <h3 className="text-lg font-bold text-white">{title}</h3>
          <button onClick={onClose} className="text-white/30 hover:text-white transition-colors bg-white/5 hover:bg-white/10 p-2 rounded-full">
            <svg className="size-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>
        <div className="p-6 overflow-y-auto">{children}</div>
        {footer && <div className="p-6 border-t border-white/5 bg-white/[0.02] flex justify-end gap-3 shrink-0">{footer}</div>}
      </div>
    </div>
  )
}

export const ConfirmModal = ({ isOpen, onClose, onConfirm, title, message, isLoading, confirmText = '确认', confirmTone = 'danger' }: any) => (
  <Modal 
    isOpen={isOpen} 
    onClose={onClose} 
    title={title}
    size="sm"
    footer={
      <>
        <button onClick={onClose} disabled={isLoading} className={cn(rdForm.button, rdForm.buttonSecondary)}>取消</button>
        <button 
          onClick={onConfirm} 
          disabled={isLoading} 
          className={cn(
            rdForm.button, 
            "min-w-[100px] text-white shadow-lg",
            confirmTone === 'danger' ? "bg-rose-500 hover:bg-rose-600 shadow-rose-500/20" : 
            confirmTone === 'primary' ? "bg-[var(--accent)] hover:opacity-90 shadow-[var(--accent)]/20" :
            "bg-emerald-500 hover:bg-emerald-600 shadow-emerald-500/20"
          )}
        >
          {isLoading ? <Spinner className="size-4 mx-auto" /> : confirmText}
        </button>
      </>
    }
  >
    <p className="text-white/60 text-sm leading-relaxed">{message}</p>
  </Modal>
)

export const Spinner = ({ className }: { className?: string }) => (
  <svg className={cn("animate-spin", className)} xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
  </svg>
)

export const FullPageLoader = ({ text = 'Loading...' }: { text?: string }) => (
  <div className="flex flex-col items-center justify-center h-64 gap-4 text-white/30">
    <Spinner className="size-8 text-[var(--accent)]" />
    <span className="text-xs font-bold uppercase tracking-widest">{text}</span>
  </div>
)
