import { Component, type ErrorInfo, type ReactNode } from 'react'
import { ApiReferenceReact } from '@scalar/api-reference-react'
import '@scalar/api-reference-react/style.css'
import { openapiReference } from './openapiManifest'

class ReferenceBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  componentDidCatch(error: Error, info: ErrorInfo) { console.error('OpenAPI reference failed', error, info) }
  render() {
    if (this.state.failed) return <div className="reference-error" role="alert"><h2>接口参考未能加载</h2><p>{openapiReference.fallbackMessage}</p><button type="button" onClick={() => window.location.reload()}>重新加载</button></div>
    return this.props.children
  }
}

export default function ReferencePage() {
  const url = new URL(openapiReference.url, window.location.href).href
  return (
    <ReferenceBoundary>
      <div className="scalar-host">
        <ApiReferenceReact configuration={{ url, theme: 'none', layout: 'modern', hideClientButton: true, hideModels: false, defaultHttpClient: { targetKey: 'shell', clientKey: 'curl' } }} />
      </div>
    </ReferenceBoundary>
  )
}
