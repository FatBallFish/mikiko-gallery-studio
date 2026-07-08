import { API_PATHS } from '../../../shared/api-types'
import { StorageConfigPage } from './StorageConfigPage'

if (API_PATHS.ops.storageConfigs !== '/api/ops/admin/v1/storage-configs') {
  throw new Error(`storage configs API path should be stable, got ${API_PATHS.ops.storageConfigs}`)
}

if (API_PATHS.ops.storageConfigDetailProbe !== '/api/ops/admin/v1/storage-configs/{storage_config_id}:probe') {
  throw new Error(`storage config probe API path should be stable, got ${API_PATHS.ops.storageConfigDetailProbe}`)
}

if (API_PATHS.ops.storageConfigSetDefault !== '/api/ops/admin/v1/storage-configs/{storage_config_id}:set-default') {
  throw new Error(`storage config set-default API path should be stable, got ${API_PATHS.ops.storageConfigSetDefault}`)
}

if (typeof StorageConfigPage !== 'function') {
  throw new Error('StorageConfigPage should be exported as a React page component')
}
