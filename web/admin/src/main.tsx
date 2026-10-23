import ReactDOM from 'react-dom/client'
import App from './App'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import './styles.css'
import '../../shared/redesign-theme.css'
import './redesign.css'
import './refactor-pages.css'
import './redesign-bridge.css'

document.documentElement.dataset.site = 'admin'
if (!document.documentElement.dataset.theme) document.documentElement.dataset.theme = 'light'

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)