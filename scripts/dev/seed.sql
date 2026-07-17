insert into user_groups (group_code, group_name, multiplier, status, description)
values
  ('basic', 'Basic', 1.00000, 'active', 'Default basic user group'),
  ('plus', 'Plus', 1.00000, 'active', 'Default plus user group'),
  ('pro', 'Pro', 1.00000, 'active', 'Default pro user group')
on conflict (group_code) do nothing;

insert into system_configs (config_category, config_key, config_value, scope, version, updated_by)
values
  ('generation_limits', 'max_image_count', '5'::jsonb, 'global', 1, 0),
  ('billing_pricing', 'points_decimal_scale', '5'::jsonb, 'global', 1, 0),
  ('billing_pricing', 'cny_per_point_micros', '312500'::jsonb, 'global', 1, 0),
  ('billing_pricing', 'auto_base_resolution_default_by_group', '{"basic":"1k","plus":"2k","pro":"4k"}'::jsonb, 'global', 1, 0)
on conflict (config_category, config_key, scope) do nothing;
