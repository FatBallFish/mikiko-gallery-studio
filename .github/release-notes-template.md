# Mikiko Gallery Studio {{VERSION}}

## 项目简介

Mikiko Gallery Studio 是一个可自部署的 AI 图片生成与管理平台，提供用户体系、积分与计费、模型路由、图片任务、公开画廊、管理后台和 OpenAPI 文档，并支持 Docker 单机、原生服务及集群部署。

## Feature 更新

{{FEATURES}}

## Bugfix

{{BUGFIXES}}

## 优化项

{{OPTIMIZATIONS}}

## 快速部署教程

准备一台已安装 Docker Engine 和 Docker Compose v2 的 Linux 或 macOS 主机，在计划保存运行文件的目录执行：

```bash
curl -fsSL https://raw.githubusercontent.com/FatBallFish/mikiko-gallery-studio/{{VERSION}}/scripts/install.sh \
  | MGSCTL_VERSION={{VERSION}} sh -s -- install --yes
```

该命令采用默认的 Docker、`full`、`single` 和 `latest` 安装方案。安装完成后，根据终端输出访问 Setup 页面并完成中间件检测与首个管理员初始化。

## 快速升级教程

升级前先备份 PostgreSQL 与对象存储。在部署主机执行：

```bash
mgsctl upgrade
mgsctl doctor
```

`mgsctl upgrade` 会解析最新 Release、校验不可变镜像与 Manifest、执行目标版本数据库迁移并滚动服务；`mgsctl doctor` 用于确认升级后的运行状态与依赖健康。

完整部署、集群和恢复说明请查看仓库中的 [`docs/runbooks/backend-deployment.md`](https://github.com/FatBallFish/mikiko-gallery-studio/blob/{{VERSION}}/docs/runbooks/backend-deployment.md)。
