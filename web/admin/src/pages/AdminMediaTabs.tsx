import React, { useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { AdminTabs } from "../components";
import { cn } from "../../../shared/classnames";
import {
  adminModelMediaHref,
  adminModelMediaTabFromHash,
  adminModelMediaTabs,
  type AdminModelMediaRoute,
  type AdminModelMediaTab,
} from "./adminModelMedia";

export function useAdminModelMediaTab() {
  const [media, setMedia] = useState<AdminModelMediaTab>(() =>
    adminModelMediaTabFromHash(window.location.hash),
  );
  useEffect(() => {
    const update = () =>
      setMedia(adminModelMediaTabFromHash(window.location.hash));
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  return media;
}

export type AdminMediaAccountCounts = Partial<
  Record<AdminModelMediaTab, number>
>;

export function AdminMediaTabs({
  route,
  value,
  counts,
}: {
  route: AdminModelMediaRoute;
  value: AdminModelMediaTab;
  counts?: AdminMediaAccountCounts;
}) {
  const items = adminModelMediaTabs.map((item) => ({
    ...item,
    badge:
      route === "access-accounts" && counts?.[item.id] !== undefined
        ? String(counts[item.id])
        : undefined,
  }));
  return (
    <AdminTabs
      ariaLabel="模型配置媒体类型"
      items={items}
      value={value}
      onChange={(next) => {
        if (next === value) return;
        window.location.hash = adminModelMediaHref(route, next).slice(1);
      }}
    />
  );
}

/* ------------------------------------------------------------------ *
 * Shared configuration form primitives.
 * These live next to the media tabs so every config page composes the
 * same single-column ledger, multi-select and sticky save bar.
 * ------------------------------------------------------------------ */

export type LedgerOption = string | { value: string; label: string };

function optionValue(option: LedgerOption) {
  return typeof option === "string" ? option : option.value;
}

function optionLabel(option: LedgerOption) {
  return typeof option === "string" ? option : option.label;
}

export function LedgerField({
  label,
  hint,
  error,
  required,
  children,
}: {
  label: string;
  hint?: string;
  error?: string | null;
  required?: boolean;
  children: ReactNode;
}) {
  const id = useId();
  const describedBy = [hint ? `${id}-hint` : '', error ? `${id}-error` : ''].filter(Boolean).join(' ') || undefined;
  const control = React.isValidElement<{ id?: string; 'aria-label'?: string; 'aria-describedby'?: string; 'aria-invalid'?: boolean }>(children) && typeof children.type === 'string' && ['input', 'select', 'textarea'].includes(children.type)
    ? React.cloneElement(children, { id: children.props.id ?? id, 'aria-label': children.props['aria-label'] ?? label, 'aria-describedby': describedBy, 'aria-invalid': Boolean(error) }) : children;
  return (
    <div className="a-field">
      <span>
        <span id={`${id}-label`}>
          {label}
          {required ? <span className="a-required"> *</span> : null}
        </span>
      </span>
      {control}
      {hint ? <small id={`${id}-hint`} className="a-hint">{hint}</small> : null}
      {error ? <em id={`${id}-error`} className="a-field__error">{error}</em> : null}
    </div>
  );
}

export function FormSection({
  index,
  title,
  description,
  children,
  aside,
  group,
}: {
  index?: number;
  title: string;
  description?: string;
  children: ReactNode;
  aside?: ReactNode;
  group?: string;
}) {
  return (
    <section className="a-form__section" data-security-editor-group={group}>
      <div className="a-form__head">
        <h3>
          {index ? <span className="a-form__num">{index}</span> : null}
          {title}
        </h3>
        {description ? <p>{description}</p> : null}
        {aside}
      </div>
      {children}
    </section>
  );
}

export function MultiSelect({
  values,
  options,
  onChange,
  placeholder = "选择或添加值",
  customPlaceholder = "自定义值",
  disabled = false,
  allowCustom = true,
  ariaLabel,
  normalize = (value: string) => value.trim(),
}: {
  values: string[];
  options: LedgerOption[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  customPlaceholder?: string;
  disabled?: boolean;
  allowCustom?: boolean;
  ariaLabel?: string;
  normalize?: (value: string) => string;
}) {
  const [open, setOpen] = useState(false);
  const [custom, setCustom] = useState("");
  const rootRef = useRef<HTMLDivElement | null>(null);
  const entries = useMemo(() => {
    const declared = options.map((option) => ({
      value: optionValue(option),
      label: optionLabel(option),
    }));
    const declaredValues = new Set(
      declared.map((option) => option.value.toLowerCase()),
    );
    const customValues = values.filter(
      (value) => !declaredValues.has(value.toLowerCase()),
    );
    return [
      ...declared,
      ...customValues.map((value) => ({ value, label: value })),
    ];
  }, [options, values]);
  const labels = useMemo(
    () =>
      new Map(
        entries.map((entry) => [entry.value.toLowerCase(), entry.label]),
      ),
    [entries],
  );

  useEffect(() => {
    if (!open) return undefined;
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return;
      setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  const addValue = () => {
    const value = normalize(custom);
    if (!value) return;
    if (!values.some((item) => item.toLowerCase() === value.toLowerCase())) {
      onChange([...values, value]);
    }
    setCustom("");
  };

  const toggle = (value: string, checked: boolean) => {
    if (checked) {
      if (!values.some((item) => item.toLowerCase() === value.toLowerCase())) {
        onChange([...values, value]);
      }
      return;
    }
    onChange(
      values.filter((item) => item.toLowerCase() !== value.toLowerCase()),
    );
  };

  const summary = values.length
    ? values.map((value) => labels.get(value.toLowerCase()) ?? value).join("、")
    : placeholder;

  return (
    <div ref={rootRef} className={cn("a-multiselect", open && "is-open")}>
      <button
        className="a-multiselect__control"
        type="button"
        aria-label={ariaLabel}
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
      >
        <span className="a-multiselect__value">{summary}</span>
        <ChevronDown aria-hidden="true" />
      </button>
      <div className="a-multiselect__panel" role="group">
        <div className="a-multiselect__options">
          {entries.map((option, index) => (
            <label key={`${option.value}-${index}`}>
              <input
                type="checkbox"
                checked={values.some(
                  (value) => value.toLowerCase() === option.value.toLowerCase(),
                )}
                disabled={disabled}
                onChange={(event) => toggle(option.value, event.target.checked)}
              />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
        {allowCustom ? (
          <div className="a-multiselect__custom">
            <input
              className="a-input"
              value={custom}
              disabled={disabled}
              placeholder={customPlaceholder}
              onChange={(event) => setCustom(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== "Enter") return;
                event.preventDefault();
                addValue();
              }}
            />
            <button
              className="a-btn a-btn--sm"
              type="button"
              disabled={disabled || !custom.trim()}
              onClick={addValue}
            >
              添加
            </button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function MultiSelectField({
  label,
  values,
  options,
  onChange,
  hint,
  error,
  required,
  placeholder,
  customPlaceholder,
  disabled,
  allowCustom,
  ariaLabel,
  normalize,
}: {
  label: string;
  values: string[];
  options: LedgerOption[];
  onChange: (values: string[]) => void;
  hint?: string;
  error?: string | null;
  required?: boolean;
  placeholder?: string;
  customPlaceholder?: string;
  disabled?: boolean;
  allowCustom?: boolean;
  ariaLabel?: string;
  normalize?: (value: string) => string;
}) {
  return (
    <LedgerField label={label} hint={hint} error={error} required={required}>
      <MultiSelect
        values={values}
        options={options}
        onChange={onChange}
        placeholder={placeholder}
        customPlaceholder={customPlaceholder}
        disabled={disabled}
        allowCustom={allowCustom}
        ariaLabel={ariaLabel ?? label}
        normalize={normalize}
      />
    </LedgerField>
  );
}

export function ChecklistField({
  label,
  values,
  options,
  onChange,
  hint,
  error,
  disabled = false,
}: {
  label: string;
  values: string[];
  options: LedgerOption[];
  onChange: (values: string[]) => void;
  hint?: string;
  error?: string | null;
  disabled?: boolean;
}) {
  return (
    <LedgerField label={label} hint={hint} error={error}>
      <div className="cfg-checklist">
        {options.map((option) => {
          const value = optionValue(option);
          return (
            <label key={value} className="a-switch">
              <input
                type="checkbox"
                checked={values.includes(value)}
                disabled={disabled}
                onChange={(event) =>
                  onChange(
                    event.target.checked
                      ? [...values, value]
                      : values.filter((item) => item !== value),
                  )
                }
              />
              <span className="a-switch__track" />
              <span>{optionLabel(option)}</span>
            </label>
          );
        })}
      </div>
    </LedgerField>
  );
}

export function SwitchField({
  label,
  checked,
  onChange,
  hint,
  disabled = false,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  hint?: string;
  disabled?: boolean;
}) {
  return (
    <div className="a-field">
      <label className="a-switch">
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={(event) => onChange(event.target.checked)}
        />
        <span className="a-switch__track" />
        <span>{label}</span>
      </label>
      {hint ? <small className="a-hint">{hint}</small> : null}
    </div>
  );
}

export function FormSaveBar({
  dirtyCount,
  errorCount,
  busy = false,
  note,
  onReset,
  onSave,
  saveLabel = "保存",
  resetLabel = "恢复",
  saveDisabled = false,
  extraActions,
}: {
  dirtyCount: number;
  errorCount: number;
  busy?: boolean;
  note?: string;
  onReset?: () => void;
  onSave: () => void;
  saveLabel?: string;
  resetLabel?: string;
  saveDisabled?: boolean;
  extraActions?: ReactNode;
}) {
  const stateLabel = errorCount
    ? `${errorCount} 项校验未通过`
    : dirtyCount
      ? `${dirtyCount} 项未保存`
      : "未修改";
  const tone = errorCount ? "a-badge--bad" : dirtyCount ? "a-badge--warn" : "a-badge--ok";
  return (
    <div className="a-form__actions">
      <div className="cfg-savebar-status">
        <span className={cn("a-badge", tone)}>
          <i />
          {stateLabel}
        </span>
        {note ? <span className="a-hint">{note}</span> : null}
      </div>
      <div className="a-row-actions">
        {extraActions}
        {onReset ? (
          <button
            className="a-btn a-btn--sm a-btn--quiet"
            type="button"
            disabled={busy || (!dirtyCount && !errorCount)}
            onClick={onReset}
          >
            {resetLabel}
          </button>
        ) : null}
        <button
          className="a-btn a-btn--sm a-btn--primary"
          type="button"
          disabled={busy || saveDisabled || errorCount > 0}
          onClick={onSave}
        >
          {busy ? "处理中..." : saveLabel}
        </button>
      </div>
    </div>
  );
}

export function FormStatusBanner({
  dirtyCount,
  errorCount,
  busy = false,
  note,
}: {
  dirtyCount: number;
  errorCount: number;
  busy?: boolean;
  note?: string;
}) {
  const text = busy
    ? "正在提交配置，请保持页面打开。"
    : errorCount
      ? `有 ${errorCount} 项校验未通过，修正后才能保存。`
      : dirtyCount
        ? `有 ${dirtyCount} 项未保存修改。`
        : "当前表单与已保存配置一致。";
  return (
    <div className="cfg-form-status" role="status">
      <span className={cn("a-badge", errorCount ? "a-badge--bad" : dirtyCount ? "a-badge--warn" : "a-badge--ok")}>
        <i />
        {text}
      </span>
      {note ? <span className="a-hint">{note}</span> : null}
    </div>
  );
}