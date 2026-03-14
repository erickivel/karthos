package configgen

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"sigs.k8s.io/yaml"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

type Generator struct {
	namespaceTemplate      *template.Template
	configMapTemplate      *template.Template
	agentDaemonSetTemplate *template.Template
	gatewayDeploymentTmpl  *template.Template
}

type configMapData struct {
	Name      string
	Namespace string
	Config    string
}

type templateData struct {
	Namespace string
}

func NewGenerator() *Generator {
	funcMap := template.FuncMap{
		"indent": indent,
	}

	return &Generator{
		namespaceTemplate:      template.Must(template.New("namespace.yaml.tmpl").Funcs(funcMap).ParseFS(otel.TemplateFS, "templates/namespace.yaml.tmpl")),
		configMapTemplate:      template.Must(template.New("configmap.yaml.tmpl").Funcs(funcMap).ParseFS(otel.TemplateFS, "templates/configmap.yaml.tmpl")),
		agentDaemonSetTemplate: template.Must(template.New("agent-daemonset.yaml.tmpl").Funcs(funcMap).ParseFS(otel.TemplateFS, "templates/agent-daemonset.yaml.tmpl")),
		gatewayDeploymentTmpl:  template.Must(template.New("gateway-deployment.yaml.tmpl").Funcs(funcMap).ParseFS(otel.TemplateFS, "templates/gateway-deployment.yaml.tmpl")),
	}
}

func (g *Generator) Generate(plan otel.Plan) ([]byte, error) {
	agentConfig, err := yaml.Marshal(plan.AgentConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal agent config: %w", err)
	}

	gatewayConfig, err := yaml.Marshal(plan.GatewayConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway config: %w", err)
	}

	var documents []string
	documents = append(documents, render(g.namespaceTemplate, templateData{Namespace: plan.Namespace}))
	documents = append(documents, render(g.configMapTemplate, configMapData{
		Name:      "otel-collector-agent-config",
		Namespace: plan.Namespace,
		Config:    string(agentConfig),
	}))
	documents = append(documents, render(g.configMapTemplate, configMapData{
		Name:      "otel-collector-gateway-config",
		Namespace: plan.Namespace,
		Config:    string(gatewayConfig),
	}))
	documents = append(documents, render(g.agentDaemonSetTemplate, templateData{Namespace: plan.Namespace}))
	documents = append(documents, render(g.gatewayDeploymentTmpl, templateData{Namespace: plan.Namespace}))

	return []byte(strings.Join(documents, "\n---\n")), nil
}

func render(tmpl *template.Template, data any) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	return strings.TrimSpace(buf.String())
}

func indent(input string, spaces int) string {
	padding := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(input, "\n"), "\n")
	for i, line := range lines {
		lines[i] = padding + line
	}
	return strings.Join(lines, "\n")
}
