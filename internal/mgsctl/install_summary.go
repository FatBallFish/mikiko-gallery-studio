package mgsctl

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func InstallSummary(plan InstallPlan, result InstallResult, revealSetupToken bool) string {
	var summary strings.Builder
	summary.WriteString("\nDeployment installed successfully.\n\n")
	fmt.Fprintf(&summary, "Runtime configuration: %s\n", result.RuntimeEnvPath)
	if revealSetupToken {
		fmt.Fprintf(&summary, "Setup token: %s\n", result.SetupToken)
	} else {
		fmt.Fprintf(&summary, "Setup token stored securely. Run mgsctl setup token show --runtime-dir %q on the deployment host to display it.\n", plan.RuntimeDir)
	}

	public := make([]serviceEndpoint, 0, 8)
	apiBase := strings.TrimRight(plan.PublicAPIURL, "/")
	setupBase := apiBase
	if apiBase == "" {
		apiBase = "http://127.0.0.1:" + plan.APIPort
		setupBase = apiBase
	} else if parsed, err := url.Parse(apiBase); err == nil {
		setupBase = parsed.Scheme + "://" + parsed.Host
	}
	if slices.Contains(plan.Components, ComponentAPI) {
		public = append(public, serviceEndpoint{"Setup", setupBase + "/setup", "Host/public access"}, serviceEndpoint{"API", apiBase + "/", "Host/public access"})
	}
	if slices.Contains(plan.Components, ComponentGateway) {
		public = append(public, serviceEndpoint{"Gateway", loopbackURL(plan.GatewayPort), "Host access"})
	}
	if slices.Contains(plan.Components, ComponentUserWeb) {
		public = append(public, serviceEndpoint{"User Web", loopbackURL(plan.UserWebPort), "Host access"})
	}
	if slices.Contains(plan.Components, ComponentAdminWeb) {
		public = append(public, serviceEndpoint{"Admin Web", loopbackURL(plan.AdminWebPort), "Host access"})
	}
	if slices.Contains(plan.Components, ComponentDocsWeb) {
		public = append(public, serviceEndpoint{"Documentation", loopbackURL(plan.DocsWebPort), "Host access"})
	}
	if slices.Contains(plan.Components, ComponentMonitoring) {
		public = append(public, serviceEndpoint{"Monitoring", loopbackURL(plan.MonitoringPort), "Host access"})
	}
	if len(public) > 0 {
		summary.WriteString("\nPublic and host endpoints:\n")
		for _, endpoint := range public {
			fmt.Fprintf(&summary, "  %-16s %-38s %s\n", endpoint.Name, endpoint.Address, endpoint.Scope)
		}
	}

	if plan.Mode == "docker" {
		internal := make([]serviceEndpoint, 0, 4)
		if slices.Contains(plan.Components, ComponentPostgres) {
			internal = append(internal, serviceEndpoint{"PostgreSQL", "postgres:5432", "Docker network only"})
		}
		if slices.Contains(plan.Components, ComponentRedis) {
			internal = append(internal, serviceEndpoint{"Redis", "redis:6379", "Docker network only"})
		}
		if slices.Contains(plan.Components, ComponentMinIO) {
			internal = append(internal, serviceEndpoint{"MinIO API", "minio:9000", "Docker network only"}, serviceEndpoint{"MinIO Console", "minio:9001", "Docker network only"})
		}
		if len(internal) > 0 {
			summary.WriteString("\nInternal middleware endpoints:\n")
			for _, endpoint := range internal {
				fmt.Fprintf(&summary, "  %-16s %-38s %s\n", endpoint.Name, endpoint.Address, endpoint.Scope)
			}
		}
	}

	if len(public) > 0 {
		summary.WriteString("\nLoopback addresses are local checks. From another machine, replace 127.0.0.1 with the deployment node IP, or use the load balancer/reverse proxy configured by the operator.\n")
	}
	summary.WriteString("\nNext steps:\n")
	step := 1
	if slices.Contains(plan.Components, ComponentAPI) {
		fmt.Fprintf(&summary, "  %d. Open %s and enter the Setup token.\n", step, setupBase+"/setup")
		step++
		fmt.Fprintf(&summary, "  %d. Complete middleware and administrator initialization, then wait for services to restart.\n", step)
		step++
		if slices.Contains(plan.Components, ComponentAdminWeb) {
			fmt.Fprintf(&summary, "  %d. Sign in through Admin Web and finish business configuration.\n", step)
			step++
		}
	} else {
		fmt.Fprintf(&summary, "  %d. Verify this node can reach the control API or external load balancer.\n", step)
		step++
		fmt.Fprintf(&summary, "  %d. Confirm node registration and health from the control node.\n", step)
		step++
	}
	fmt.Fprintf(&summary, "  %d. Check health with mgsctl status --runtime-dir %q and mgsctl doctor --runtime-dir %q.\n", step, plan.RuntimeDir, plan.RuntimeDir)
	return summary.String()
}

type serviceEndpoint struct {
	Name    string
	Address string
	Scope   string
}

func loopbackURL(port string) string {
	if port == "80" || port == "" {
		return "http://127.0.0.1/"
	}
	return "http://127.0.0.1:" + port + "/"
}
