package deployments

import _ "embed"

var (
	//go:embed docker-compose/docker-compose.prod.yml
	dockerCompose []byte

	//go:embed docker-compose/minio-init.sh
	minioInit []byte

	//go:embed docker-compose/postgres-init.sh
	postgresInit []byte

	//go:embed nginx/default.conf
	nginxDefault []byte

	//go:embed monitoring/prometheus.yml
	prometheus []byte
)

func DockerCompose() []byte { return append([]byte(nil), dockerCompose...) }
func MinIOInit() []byte     { return append([]byte(nil), minioInit...) }
func PostgresInit() []byte  { return append([]byte(nil), postgresInit...) }
func NginxDefault() []byte  { return append([]byte(nil), nginxDefault...) }
func Prometheus() []byte    { return append([]byte(nil), prometheus...) }
