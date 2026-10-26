// Exact result hosts shown in Alibaba Cloud's official video API examples:
// https://help.aliyun.com/zh/model-studio/text-to-video-api-reference
// Suggestions are never selected automatically; other providers use custom hosts.
export const videoArtifactHostSuggestions = [
  "dashscope-result-sh.oss-accelerate.aliyuncs.com",
  "dashscope-a717.oss-accelerate.aliyuncs.com",
];

export function videoArtifactHosts(extra?: Record<string, unknown>): string[] {
  const value = extra?.video_artifact_hosts;
  return Array.isArray(value) ? [...new Set(value.filter((item): item is string => typeof item === "string").map((item) => item.trim().toLowerCase()).filter(Boolean))] : [];
}

export function videoArtifactHostsEnabled(extra?: Record<string, unknown>): boolean {
  if (extra && Object.prototype.hasOwnProperty.call(extra, "video_artifact_hosts_enabled")) return extra.video_artifact_hosts_enabled !== false;
  return videoArtifactHosts(extra).length > 0;
}

export function normalizeVideoArtifactHost(value: string): string {
  const host = value.trim().toLowerCase();
  if (!host || /[\s/@?#*]/.test(host)) return "";
  try {
    const parsed = new URL(`https://${host}`);
    return parsed.host === host && !parsed.username && !parsed.password ? host : "";
  } catch { return ""; }
}
