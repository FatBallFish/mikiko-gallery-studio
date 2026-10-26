import { FormSection, LedgerField, MultiSelectField, ChecklistField } from './AdminMediaTabs'
import { configObject, configValue, patchConfig, summarizeConfig, type ConfigObject } from './nativeImageConfig'
import { nativeImageProfile, type NativeImageProfile } from '../../../shared/native-image'

type Props = { extra?: ConfigObject; inherited?: ConfigObject; adapter: string; profile?: NativeImageProfile; onChange: (extra: ConfigObject) => void }
export function ImageConfigurationFields({ extra, inherited, adapter, profile, onChange }: Props) {
  const native = adapter === 'qwen_image' || adapter === 'seedream_image'
  const official = profile ? nativeImageProfile(profile.model) : undefined
  const patchProfile = (key: string, value: unknown) => {
    if (!profile) return
    const existing = configValue(extra, ['native_image', 'profile'])
    onChange(patchConfig(extra, ['native_image', 'profile'], { ...(existing === undefined || existing === null ? profile : configObject(existing)), [key]: value }))
  }
  const select = (label: string, path: string[], options: Array<[string, string]>, hint: string) => {
    const value = configValue(extra, path)
    const inheritedValue = configValue(inherited, path)
    const encoded = value === undefined ? '__inherit' : value === null ? '__null' : String(value)
    return <LedgerField key={path.join('.')} label={label} hint={`${hint} 上级配置：${inheritedValue === undefined ? '系统默认' : summarizeConfig(inheritedValue)}。`}>
      <select className="a-select" aria-label={label} value={encoded} onChange={(event) => {
        const text = event.target.value
        onChange(patchConfig(extra, path, text === '__inherit' ? undefined : text === '__null' ? null : text === 'true' ? true : text === 'false' ? false : text))
      }}>
        <option value="__inherit">继承 / 系统默认</option>
        {value === null ? <option value="__null">保留现有 null</option> : null}
        {value !== undefined && value !== null && !options.some(([key]) => key === String(value)) ? <option value={String(value)}>保留现有值：{String(value)}</option> : null}
        {options.map(([key, text]) => <option key={key} value={key}>{text}</option>)}
      </select>
    </LedgerField>
  }
  return <>
    <FormSection title="平台默认" description="只修改已操作的字段；继承表示移除本层覆盖，已有 null 会原样保留。用户显式 API 参数按服务端规则覆盖平台默认。">
      {!native ? <div className="a-form__pair">
        {select('来源模式 source_mode', ['source_mode'], [['images', 'Images API'], ['codex_responses', 'Codex Responses']], '三态：继承、Images、Codex；Codex 强制 quality=auto。旧版 gpt_image_2_codex_source 独立保留，true 时仍可启用 Codex。')}
        {select('发送 response_format', ['gpt_image_response_format'], [['true', '发送'], ['false', '不发送']], '三态：继承、true、false；适用于 GPT Image，继承时系统默认发送。现有服务端优先采用账号覆盖，再读取模型配置。')}
      </div> : <>
        <p className="a-hint">watermark 默认 false：关闭厂商官方 AI 生成标记，不提供自定义水印能力。未显式覆盖的新请求默认 false；API 显式参数按服务端策略处理。</p>
        {adapter === 'qwen_image' ? <>
          <LedgerField label="可选固定随机种子" hint="仅 Qwen 支持，0 是有效固定种子；留空删除本层固定值并继承上级，上级也未配置时每次随机。随机种子不向用户展示。">
            <input className="a-input" aria-label="可选固定随机种子" type="number" min="0" max="2147483647" value={typeof configValue(extra, ['native_image', 'defaults', 'seed']) === 'number' ? String(configValue(extra, ['native_image', 'defaults', 'seed'])) : ''} onChange={(event) => onChange(patchConfig(extra, ['native_image', 'defaults', 'seed'], event.target.value === '' ? undefined : event.target.valueAsNumber))} />
            {configValue(extra, ['native_image', 'defaults', 'seed']) === null ? <small className="a-hint">现有 null 保持原样；输入后才会覆盖。</small> : null}
          </LedgerField>
          <div className="a-form__pair">
            {select('Qwen 提示词优化', ['native_image', 'defaults', 'qwen', 'prompt_extend'], [['false', '关闭'], ['true', '开启（需核实费用）']], '收费资料未知，系统默认关闭；管理方须核实费用后开启。')}
            {select('Qwen 思考', ['native_image', 'defaults', 'qwen', 'enable_thinking'], [['false', '关闭'], ['true', '开启（需核实费用）']], '收费资料未知，系统默认关闭；管理方须核实费用后开启。')}
          </div>
          {select('Qwen 优化交互模式', ['native_image', 'defaults', 'qwen', 'prompt_extend_mode'], [['direct', 'Direct 直接执行']], '平台仅采用 direct；Agent 不作为平台默认。')}
        </> : select('Seedream 提示词优化', ['native_image', 'defaults', 'seedream', 'optimize_prompt_mode'], [['standard', 'Standard 标准'], ...(profile?.optimize_prompt_modes?.includes('fast') ? [['fast', 'Fast 快速'] as [string, string]] : [])], '系统默认 standard；厂商没有 off 模式，fast 仅 Pro 型号可用。收费资料未知，切换前需管理方核实费用。')}
      </>}
    </FormSection>
    <FormSection title="配置摘要" description="只读显示已有扩展配置（包括未知深层键）；保存不会用表单默认值整体替换。">
      {native ? <LedgerField label="能力 profile 覆盖" hint="保持未编辑时的继承状态；编辑任一限制才创建本层完整覆盖。只允许收紧官方能力。切换型号时保留未知字段。">
        {profile && official ? <>
          <p className="a-hint">{profile.model} · 总像素 {profile.min_pixels}–{profile.max_pixels} · 参考图最多 {profile.max_reference_images} · 出图最多 {profile.max_output_images}</p>
          <div className="a-form__pair">
            {([
              ['max_reference_images', '最多参考图', 0, official.max_reference_images],
              ['max_output_images', '最多出图数', 1, official.max_output_images],
              ['min_pixels', '最小总像素', official.min_pixels, profile.max_pixels],
              ['max_pixels', '最大总像素', profile.min_pixels, official.max_pixels],
              ['max_aspect_ratio', '最大宽高比', 1, official.max_aspect_ratio],
              ...(official.max_layers !== undefined ? [['max_layers', '最大图层数', 1, official.max_layers] as [keyof NativeImageProfile, string, number, number]] : []),
            ] as Array<[keyof NativeImageProfile, string, number, number]>).map(([key, label, min, max]) => <LedgerField key={key} label={label} hint={`官方允许范围 ${min}–${max}；仅可收紧。`}><input className="a-input" type="number" min={min} max={max} step={key === 'max_aspect_ratio' ? 'any' : '1'} value={String(profile[key] ?? '')} onChange={(event) => patchProfile(key, event.target.value === '' ? undefined : event.target.valueAsNumber)} /></LedgerField>)}
          </div>
          <ChecklistField label="允许操作" values={profile.operations} options={official.operations.map((value) => ({ value, label: value === 'generate' ? '生成' : value === 'edit' ? '参考图编辑' : '拆层（仍需核实价格）' }))} onChange={(operations) => onChange(patchConfig(extra, ['native_image', 'profile'], { ...profile, ...configObject(configValue(extra, ['native_image', 'profile'])), operations, layer_decomposition: operations.includes('layer_decomposition') && official.layer_decomposition }))} />
          {official.layer_size_modes ? <MultiSelectField label="拆层尺寸模式" values={profile.layer_size_modes ?? []} options={official.layer_size_modes} allowCustom={false} onChange={(modes) => patchProfile('layer_size_modes', modes)} /> : null}
          <MultiSelectField label="允许输出格式" values={profile.output_formats} options={official.output_formats} allowCustom={false} onChange={(formats) => patchProfile('output_formats', formats)} />
        </> : <p className="a-hint">选择精确型号后可配置能力；现有覆盖值保留。</p>}
        {configValue(extra, ['native_image', 'profile']) !== undefined ? <button type="button" className="a-btn a-btn--sm" onClick={() => { if (window.confirm('恢复继承会移除当前模型的 profile 覆盖。确认恢复？')) onChange(patchConfig(extra, ['native_image', 'profile'], undefined)) }}>恢复 profile 继承</button> : null}
      </LedgerField> : null}
      <details><summary>查看保留的扩展字段</summary><ConfigTree value={configObject(extra)} /></details>
    </FormSection>
  </>
}

function ConfigTree({ value }: { value: unknown }) {
  if (value === null || typeof value !== 'object') return <span className="a-hint">{value === null ? 'null（显式空值）' : String(value)}</span>
  return <dl className="cfg-config-tree">{Object.entries(value).map(([key, child]) => <div key={key}><dt>{key}</dt><dd>{child !== null && typeof child === 'object' ? <details><summary>{Array.isArray(child) ? `${child.length} 项` : `${Object.keys(child).length} 个字段`}</summary><ConfigTree value={child} /></details> : <ConfigTree value={child} />}</dd></div>)}</dl>
}
