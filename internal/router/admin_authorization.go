package router

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

const adminCasbinModel = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = (g(r.sub, p.sub, r.dom) || r.sub == p.sub) && r.dom == p.dom && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
`

type adminAuthorizer struct {
	enforcer *casbin.Enforcer
}

func newAdminAuthorizer(cfg AdminAuthorizationConfig) (*adminAuthorizer, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	m, err := model.NewModelFromString(adminCasbinModel)
	if err != nil {
		return nil, err
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, err
	}
	for _, raw := range cfg.Policy {
		fields, err := parseCasbinPolicyLine(raw)
		if err != nil {
			return nil, err
		}
		switch fields[0] {
		case "p":
			if _, err := enforcer.AddPolicy(fields[1], fields[2], fields[3], fields[4]); err != nil {
				return nil, err
			}
		case "g":
			if _, err := enforcer.AddGroupingPolicy(fields[1], fields[2], fields[3]); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported policy line %q", fields[0])
		}
	}
	return &adminAuthorizer{enforcer: enforcer}, nil
}

func parseCasbinPolicyLine(raw string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(raw))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	fields, err := reader.Read()
	if err != nil {
		return nil, err
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields, nil
}

func (a *adminAuthorizer) enforce(subject adminBasicRuntime, object, action string) bool {
	if a == nil || a.enforcer == nil {
		return false
	}
	ok, err := a.enforcer.Enforce(subject.subject, subject.domain, object, action)
	return err == nil && ok
}
