import ReactDOM from 'react-dom/client'
import App from './App'
import './styles.css'
import '../../shared/redesign-theme.css'
import './redesign.css'
import './refactor-pages.css'
import './redesign-bridge.css'

document.documentElement.dataset.site = 'user'
if (!document.documentElement.dataset.theme) document.documentElement.dataset.theme = 'dark'

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)