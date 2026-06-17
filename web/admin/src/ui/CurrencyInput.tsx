import { useId } from 'react'
import { adminCurrencyOptions, normalizeAdminCurrency } from './currency'

export { adminCurrencyOptions, normalizeAdminCurrency }

export function CurrencyInput({
  value,
  onChange,
  placeholder = 'CNY',
  disabled = false,
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  disabled?: boolean
}) {
  const listId = useId()
  return (
    <>
      <input
        value={value}
        list={listId}
        onChange={(event) => onChange(event.target.value)}
        onBlur={(event) => onChange(normalizeAdminCurrency(event.target.value))}
        placeholder={placeholder}
        disabled={disabled}
        spellCheck={false}
      />
      <datalist id={listId}>
        {adminCurrencyOptions.map((currency) => <option key={currency} value={currency} />)}
      </datalist>
    </>
  )
}
