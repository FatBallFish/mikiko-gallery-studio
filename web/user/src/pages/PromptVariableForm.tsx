import { AlertCircle } from 'lucide-react'
import { promptVariableNames } from './promptTemplateEditorModel'

export function PromptVariableForm({ template, values, disabled = false, onChange }: {
  template: string
  values: Readonly<Record<string, string>>
  disabled?: boolean
  onChange: (name: string, value: string) => void
}) {
  const names = promptVariableNames(template)
  if (!names.length) return null
  return (
    <fieldset className="prompt-variable-form" disabled={disabled}>
      <legend>模板变量</legend>
      <div className="prompt-variable-grid">
        {names.map((name) => {
          const value = values[name] ?? ''
          const invalid = !value.trim()
          return (
            <label key={name} className="prompt-variable-field">
              <span>{name}{invalid ? <AlertCircle size={14} aria-label="尚未填写" /> : null}</span>
              <textarea
                value={value}
                maxLength={4000}
                rows={2}
                aria-invalid={invalid}
                placeholder={`填写“${name}”的内容`}
                onChange={(event) => onChange(name, event.target.value)}
              />
            </label>
          )
        })}
      </div>
    </fieldset>
  )
}
