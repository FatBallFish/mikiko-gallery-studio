import type { ModelAccount, ModelAccountModel } from '../../../shared/api-types'

export type AdminModelMediaTab = "image" | "video" | "audio" | "text";
export type AdminModelMediaRoute = "access-accounts" | "routing" | "pricing";

export const adminModelMediaTabs: Array<{
  id: AdminModelMediaTab;
  label: string;
  description: string;
}> = [
  { id: "image", label: "图片", description: "图片生成账号、模型与价格" },
  { id: "video", label: "视频", description: "视频生成账号、模型与参数价格" },
  { id: "audio", label: "音频", description: "音频能力预留" },
  { id: "text", label: "文本", description: "提示词优化模型与成本" },
];

const mediaValues = new Set<AdminModelMediaTab>(
  adminModelMediaTabs.map((item) => item.id),
);

export function adminModelMediaTabFromHash(hash: string): AdminModelMediaTab {
  const query = hash.includes("?") ? hash.slice(hash.indexOf("?") + 1) : "";
  const value = new URLSearchParams(query).get(
    "media",
  ) as AdminModelMediaTab | null;
  return value && mediaValues.has(value) ? value : "image";
}

export function adminModelMediaHref(
  route: AdminModelMediaRoute,
  media: AdminModelMediaTab,
) {
  return `#/${route}?media=${media}`;
}

export function modelAccountMediaType(
  account: Pick<ModelAccount, 'adapter_type' | 'extra'>,
): AdminModelMediaTab {
  const explicit = account.extra?.media_type;
  if (explicit === 'image' || explicit === 'video' || explicit === 'audio' || explicit === 'text') return explicit;
  return account.adapter_type === 'seedance' || account.adapter_type === 'minimax' ? 'video' : 'image';
}

export function modelMediaType(
  model: Pick<ModelAccountModel, 'extra'>,
  account: Pick<ModelAccount, 'adapter_type' | 'extra'>,
): AdminModelMediaTab {
  const explicit = model.extra?.media_type;
  if (explicit === 'image' || explicit === 'video' || explicit === 'audio' || explicit === 'text') return explicit;
  return modelAccountMediaType(account);
}
