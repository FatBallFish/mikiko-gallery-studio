// @ts-nocheck
import fs from 'node:fs'
import { adminModelMediaHref, adminModelMediaTabFromHash, adminModelMediaTabs, modelAccountMediaType, modelMediaType } from './adminModelMedia'

if (adminModelMediaTabs.map((item) => item.id).join(',') !== 'image,video,audio,text') throw new Error('model administration tabs must keep image/video/audio/text order')
if (adminModelMediaTabFromHash('#/routing?media=video') !== 'video') throw new Error('media query must survive route refresh')
if (adminModelMediaTabFromHash('#/pricing?media=unknown') !== 'image') throw new Error('unknown media query must fall back to image')
if (adminModelMediaHref('access-accounts', 'text') !== '#/access-accounts?media=text') throw new Error('media tab href must preserve page route')
if (modelAccountMediaType({ adapter_type: 'seedance', extra: {} } as any) !== 'video') throw new Error('legacy Seedance accounts must remain video accounts')
if (modelAccountMediaType({ adapter_type: 'openai_compatible', extra: { media_type: 'video' } } as any) !== 'video') throw new Error('explicit account media type must take priority')
if (modelMediaType({ extra: { media_type: 'video' } } as any, { adapter_type: 'openai_compatible', extra: {} } as any) !== 'video') throw new Error('model media type must override its account default')
if (modelMediaType({ extra: {} } as any, { adapter_type: 'openai_compatible', extra: {} } as any) !== 'image') throw new Error('legacy image models must remain image models')

for (const file of ['ProviderModelsPage.tsx', 'RoutingPage.tsx', 'PricingPage.tsx']) {
  const source = fs.readFileSync(new URL(`./${file}`, import.meta.url), 'utf8')
  if (!source.includes('AdminMediaTabs')) throw new Error(`${file} must render the shared media tabs`)
}

const settings = fs.readFileSync(new URL('./SystemSettingsPage.tsx', import.meta.url), 'utf8')
if (settings.includes("id: 'text-models'")) throw new Error('text model configuration must leave system settings')
if (settings.includes('通用、安全、存储和文本模型配置聚合')) throw new Error('system settings description must not advertise the migrated text model page')
