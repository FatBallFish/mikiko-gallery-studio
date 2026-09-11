# 拆分 mgsctl 与应用镜像发布流水线需求

> 日期：2026-09-11
> 状态：已确认（仓库所有者提出）
> 线上基线：统一发布流水线 `.github/workflows/release.yml`，单一 `vX.Y.Z` tag 同时产出 mgsctl 二进制与五个应用镜像

## 背景

当前一个 `v*` tag 触发的发布会同时产出 mgsctl 二进制、原生/后端/前端安装包、五个 Docker 镜像和 release manifest，两者版本被绑定为同一个 tag。实际迭代中经常只改动了应用侧代码（镜像需要新版本），mgsctl 代码没有变化，却也会随发布产生一个新版本的 mgsctl 二进制。这导致：

- mgsctl 版本号噪声：无意义的版本递增，难以判断 mgsctl 是否真的变更。
- 已部署环境 `mgsctl self-update` 后版本变化但行为无差异，运维核对成本高。

## 目标

1. 拆分为两条独立流水线：
   - 应用发布：`vX.Y.Z` tag 触发，产出五个 Docker 镜像、原生/后端/前端安装包与 release manifest，**不再产出 mgsctl 二进制**。
   - mgsctl 发布：`mgsctl-vX.Y.Z` tag 触发，仅产出六平台 mgsctl 二进制与校验和，发布独立的 GitHub Release。
2. 两条流水线版本完全独立，各自递增，互不强制同步。
3. 现有消费路径保持可用：
   - `scripts/install.sh` 默认下载最新 mgsctl 二进制。
   - `mgsctl self-update`（含线上已部署旧版本）解析最新 mgsctl。
   - `mgsctl install/upgrade --image-tag latest` 解析最新应用 release manifest。
4. 显式版本选择沿用 tag 名：应用 `vX.Y.Z`，mgsctl `mgsctl-vX.Y.Z`。

## 验收条件

- 推送 `v*` tag 不再构建/上传任何 `mgsctl-*` 资产。
- 推送 `mgsctl-v*` tag 只构建/上传 mgsctl 资产，不构建镜像与安装包。
- `releases/latest/download/mgsctl-<os>-<arch>` 始终指向最新 mgsctl 发布（应用 Release 不抢占 GitHub Latest 标记）。
- mgsctl 解析应用 `latest` manifest 时不再依赖 GitHub Latest Release，而是选取最新的、携带 `release-manifest.json` 资产的 Release；找不到时回退旧的 `latest/download` 路径（兼容历史统一发布）。
- 仓库合同测试（release contract、install wrapper contract）与 `verify.sh` 全部通过，并覆盖新流水线的关键约束。
- 文档（部署 runbook、devops README）说明新的双 tag 发布流程与旧版本兼容性注意事项。

## 非目标

- 不迁移仓库、不改镜像仓库地址。
- 不为历史统一 Release 补发或重命名资产。
- 不引入 monorepo 版本工具；tag 仍手工推送。
- 不改变 release manifest 的 schema（仍为 schema_version 1）。
