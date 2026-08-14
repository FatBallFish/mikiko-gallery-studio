import { useEffect, useState } from "react";
import { AdminTabs } from "../components";
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

export function AdminMediaTabs({
  route,
  value,
}: {
  route: AdminModelMediaRoute;
  value: AdminModelMediaTab;
}) {
  return (
    <AdminTabs
      ariaLabel="模型配置媒体类型"
      items={adminModelMediaTabs}
      value={value}
      onChange={(next) => {
        if (next === value) return;
        window.location.hash = adminModelMediaHref(route, next).slice(1);
      }}
    />
  );
}
