-- Pic Gallery initial schema bootstrap.

create table if not exists user_groups (
  id bigserial primary key,
  group_code varchar(32) not null unique,
  group_name varchar(64) not null,
  multiplier numeric(20,5) not null default 1.00000,
  status varchar(16) not null default 'active',
  description varchar(255),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists users (
  id bigserial primary key,
  email varchar(255) not null unique,
  password_hash varchar(255),
  nickname varchar(64) not null default '',
  bio varchar(255) not null default '',
  avatar_object_key varchar(255),
  status varchar(32) not null default 'pending',
  user_group_id bigint not null default 0,
  token_version int not null default 0,
  default_locale varchar(16) not null default 'zh-CN',
  theme varchar(16) not null default 'system',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table if not exists admin_users (
  id bigserial primary key,
  email varchar(255) not null unique,
  password_hash varchar(255) not null,
  role varchar(32) not null default 'admin',
  status varchar(32) not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists refresh_sessions (
  id uuid primary key,
  user_id bigint not null,
  session_family_id uuid not null,
  refresh_token_hash varchar(128) not null unique,
  status varchar(32) not null default 'active',
  user_agent varchar(255) not null default '',
  ip_addr inet,
  expires_at timestamptz not null,
  last_rotated_at timestamptz,
  replaced_by_session_id uuid,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists api_keys (
  id bigserial primary key,
  user_id bigint not null,
  access_key varchar(64) not null unique,
  secret_hash varchar(128) not null,
  secret_ciphertext varchar(512),
  name varchar(64) not null,
  status varchar(32) not null default 'active',
  group_code varchar(32) not null default 'default',
  total_quota_points numeric(20,5),
  daily_quota_points numeric(20,5),
  total_quota_used_points numeric(20,5) not null default 0.00000,
  daily_quota_used_points numeric(20,5) not null default 0.00000,
  quota_usage_day varchar(10),
  rpm_limit int,
  rpm_window_started_at timestamptz,
  rpm_window_count int not null default 0,
  expires_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table if not exists api_key_quota_reservations (
  id bigserial primary key,
  api_key_id bigint not null,
  reservation_id varchar(128) not null,
  points numeric(20,5) not null,
  usage_day varchar(10) not null,
  status varchar(16) not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (api_key_id, reservation_id)
);

create index if not exists apikey_user_id on api_keys (user_id);
create index if not exists apikey_status on api_keys (status);
create index if not exists apikey_group_code on api_keys (group_code);
create index if not exists apikeyquotareservation_api_key_id_status on api_key_quota_reservations (api_key_id, status);

create table if not exists redeem_codes (
  id bigserial primary key,
  batch_id bigint not null default 0,
  code varchar(64) not null unique,
  status varchar(32) not null default 'inactive',
  reward_type varchar(16) not null default 'points',
  reward_value numeric(20,5) not null default 0.00000,
  valid_from timestamptz not null,
  valid_until timestamptz not null,
  max_redemptions int not null default 1,
  redeemed_count int not null default 0,
  last_redeemed_by bigint,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists point_ledgers (
  id bigserial primary key,
  user_id bigint not null,
  task_id uuid,
  order_id bigint,
  redeem_code_id bigint,
  ledger_type varchar(32) not null,
  change_points numeric(20,5) not null default 0.00000,
  balance_after numeric(20,5) not null default 0.00000,
  frozen_after numeric(20,5) not null default 0.00000,
  reason varchar(255) not null default '',
  operator_admin_id bigint,
  idempotency_key varchar(128) unique,
  created_at timestamptz not null default now()
);

create table if not exists model_providers (
  id bigserial primary key,
  provider_code varchar(64) not null unique,
  provider_type varchar(32) not null,
  auth_config_encrypted text not null default '',
  health_status varchar(32) not null default 'unknown',
  enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists model_routes (
  id bigserial primary key,
  group_code varchar(32) not null,
  task_type varchar(32) not null,
  provider_model_id bigint not null default 0,
  priority int not null default 0,
  weight_percent int not null default 100,
  fallback_order int not null default 0,
  enabled boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists provider_error_policies (
  id bigserial primary key,
  provider_type varchar(32) not null,
  http_status int not null default 0,
  provider_error_code varchar(64) not null default '',
  action varchar(32) not null,
  platform_error_code varchar(64) not null,
  retry_budget int not null default 0,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists system_configs (
  id bigserial primary key,
  config_category varchar(32) not null,
  config_key varchar(64) not null,
  config_value jsonb not null,
  scope varchar(16) not null default 'global',
  version bigint not null default 1,
  updated_by bigint not null default 0,
  updated_at timestamptz not null default now(),
  unique (config_category, config_key, scope)
);

create table if not exists reference_assets (
  id uuid primary key,
  user_id bigint not null,
  api_key_id bigint,
  upload_source varchar(16) not null default 'web',
  status varchar(32) not null default 'uploading',
  storage_driver varchar(16) not null default 'local',
  object_key varchar(255) not null unique,
  mime_type varchar(64) not null,
  file_size_bytes bigint not null default 0,
  width int,
  height int,
  sha256 varchar(64) not null,
  bound_task_id uuid,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table if not exists image_tasks (
  id uuid primary key,
  user_id bigint not null,
  api_key_id bigint,
  source_channel varchar(16) not null default 'web',
  task_type varchar(32) not null,
  status varchar(32) not null default 'queued',
  progress_stage varchar(32) not null default '',
  progress_message text not null default '',
  prompt text not null,
  negative_prompt text,
  abstract_model varchar(64) not null,
  size_mode varchar(16) not null default 'ratio',
  base_resolution varchar(16) not null default 'auto',
  quality varchar(16) not null default 'auto',
  requested_size varchar(32),
  resolved_width int,
  resolved_height int,
  aspect_ratio varchar(16) not null default '1:1',
  output_format varchar(16) not null default 'png',
  output_compression int not null default 100,
  moderation varchar(16) not null default 'auto',
  requested_output_image_count int not null default 1,
  success_output_image_count int not null default 0,
  reference_image_count int not null default 0,
  mask_present boolean not null default false,
  reference_strength int,
  seed bigint,
  response_mode varchar(16) not null default 'async',
  save_policy varchar(16) not null default 'private',
  estimated_points numeric(20,5) not null default 0.00000,
  actual_points numeric(20,5) not null default 0.00000,
  pricing_snapshot jsonb not null default '{}'::jsonb,
  routing_snapshot jsonb not null default '{}'::jsonb,
  error_policy_snapshot jsonb not null default '{}'::jsonb,
  provider_trace jsonb not null default '{}'::jsonb,
  lease_owner varchar(64),
  lease_expires_at timestamptz,
  error_code varchar(64),
  error_message text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);

create table if not exists task_images (
  id uuid primary key,
  task_id uuid not null,
  user_id bigint not null,
  image_role varchar(16) not null default 'output',
  storage_driver varchar(16) not null default 'local',
  object_key varchar(255) not null unique,
  mime_type varchar(64) not null,
  file_size_bytes bigint not null default 0,
  width int not null default 0,
  height int not null default 0,
  sha256 varchar(64) not null,
  visibility_status varchar(32) not null default 'private',
  review_reason varchar(255),
  published_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists audit_logs (
  id bigserial primary key,
  actor_type varchar(16) not null,
  actor_id varchar(64) not null,
  action varchar(64) not null,
  target_type varchar(32) not null,
  target_id varchar(64) not null,
  result varchar(16) not null default 'success',
  metadata jsonb not null default '{}'::jsonb,
  ip_addr inet,
  user_agent varchar(255) not null default '',
  created_at timestamptz not null default now()
);
