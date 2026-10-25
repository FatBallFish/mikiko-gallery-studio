import { PromptTemplateEditor } from '../../pages/PromptTemplateEditor'

export function NegativePromptControl({ supported, value, disabled, onChange }: { supported?: boolean; value: string; disabled?: boolean; onChange: (value: string) => void }) {
  if (!supported) return null
  return <details className="native-negative-prompt"><summary>负向提示词{value ? ' · 已填写' : ' · 可选'}</summary><div className="u-prompt-box"><PromptTemplateEditor value={value} assets={[]} variables={{}} disabled={disabled} onChange={onChange} placeholder="描述希望避免的画面元素…" /></div></details>
}
