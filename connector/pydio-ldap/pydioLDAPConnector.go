package pydio_ldap

import (
	"github.com/coreos/dex/connector"
	"github.com/Sirupsen/logrus"
	"github.com/pydio/poc/lib-pydio-ldap"
	"fmt"
	"context"
	"gopkg.in/ldap.v2"
	"strings"
)

type Config struct {
	lib_pydio_ldap.Config
}

func (c *Config) Open(logger logrus.FieldLogger) (connector.Connector, error) {
	return c.OpenConnector(logger)
}

func (c *Config) OpenConnector(logger logrus.FieldLogger) (interface {
	connector.Connector
	connector.PasswordConnector
	connector.RefreshConnector
}, error) {
	return c.openConnector(logger)
}

func (c *Config) openConnector(logger logrus.FieldLogger) (*PydioLDAPConnector, error) {
	return &PydioLDAPConnector{*c, logger}, nil
}

type PydioLDAPConnector struct {
	Config
	logger logrus.FieldLogger
}

var (
	_ connector.PasswordConnector = (*PydioLDAPConnector)(nil)
	_ connector.RefreshConnector  = (*PydioLDAPConnector)(nil)
)

func (p *PydioLDAPConnector) Login(ctx context.Context, s connector.Scopes, username, password string) (identity connector.Identity, validPassword bool, err error) {
	p.logger.Printf("Login request for User:%s", username)
	identity = connector.Identity{
		UserID:        "",
		Username:      "",
		Email:         "",
		EmailVerified: true,

		Groups:        []string{},
		ConnectorData: nil,
	}

	conf := p.Config

	var logger logrus.FieldLogger
	server, err := conf.OpenConnection(logger)
	if err != nil {
		fmt.Println("Error: %v", err)
	}
	ok, err := server.CheckPassword(username, password)
	if err != nil {
		return connector.Identity{}, false, err
	}
	if !ok {
		return connector.Identity{}, false, fmt.Errorf("Login failed")
	}

	rules := conf.Config.MappingRules.Rules
	defaultRules := []lib_pydio_ldap.MappingRule{}
	defaultRule := lib_pydio_ldap.MappingRule{
		RuleName:       "ldapDefaultRule01",
		LeftAttribute:  "UserID",
		RightAttribute: conf.User.IDAttribute,
		RuleString:     "",
		RolePrefix:     "",
	}
	defaultRuleSource := lib_pydio_ldap.MappingRule{
		RuleName:       "ldapDefaultRuleAuthSource",
		LeftAttribute:  "AuthSource",
		RightAttribute: "ldap",
		RuleString:     "",
		RolePrefix:     "",
	}
	defaultRules = append(defaultRules, defaultRule)
	defaultRules = append(defaultRules, defaultRuleSource)

	if len(rules) > 0 {
		for _, rule := range rules {
			defaultRules = append(defaultRules, rule)
		}
	}

	expected := []string{}
	if len(defaultRules) > 0 {
		for _, rule := range defaultRules {
			expected = append(expected, rule.RightAttribute)
		}
	}

	fullAttributeUser, err := server.GetUser(username, expected)
	// TODO Check scope
	ident, err := p.MapUser(defaultRules, fullAttributeUser)
	if err != nil {
		return connector.Identity{}, false, err
	}

	return ident, true, nil
}

func (p *PydioLDAPConnector) Refresh(ctx context.Context, s connector.Scopes, ident connector.Identity) (connector.Identity, error) {
	p.logger.Printf("Refresh request for User ID: %s", ident.UserID)

	conf := p.Config

	var logger logrus.FieldLogger
	server, err := conf.OpenConnection(logger)

	if err != nil {
		fmt.Println("Error: %v", err)
	}

	rules := conf.MappingRules.Rules

	defaultRules := []lib_pydio_ldap.MappingRule{}
	defaultRule := lib_pydio_ldap.MappingRule{
		RuleName:       "ldapDefaultRule01",
		LeftAttribute:  "UserID",
		RightAttribute: conf.User.IDAttribute,
		RuleString:     "",
		RolePrefix:     "",
	}
	defaultRuleSource := lib_pydio_ldap.MappingRule{
		RuleName:       "ldapDefaultRuleAuthSource",
		LeftAttribute:  "AuthSource",
		RightAttribute: "ldap",
		RuleString:     "",
		RolePrefix:     "",
	}
	defaultRules = append(defaultRules, defaultRule)
	defaultRules = append(defaultRules, defaultRuleSource)

	if len(rules) > 0 {
		for _, rule := range rules {
			defaultRules = append(defaultRules, rule)
		}
	}

	expected := []string{}
	if len(defaultRules) > 0 {
		for _, rule := range defaultRules {
			expected = append(expected, rule.RightAttribute)
		}
	}

	fullAttributeUser, err := server.GetUser(ident.UserID, expected)

	// TODO Check scope
	newIdent, err := p.MapUser(defaultRules, fullAttributeUser)
	if err != nil {
		return connector.Identity{}, err
	}
	return newIdent, nil
}

func (p *PydioLDAPConnector) MapUser(ruleSet []lib_pydio_ldap.MappingRule, user *ldap.Entry) (ident connector.Identity, err error) {
	//ident = connector.Identity{}
	if len(ruleSet) > 0 {
		for _, rule := range ruleSet {
			rightValues := user.GetAttributeValues(rule.RightAttribute)
			if rightValues != nil {
				if p.Config.UserAttributeMeaningMemberOf == rule.RightAttribute {
					rightValues = rule.ConvertDNtoName(rightValues)
				}

				rightValues = rule.RemoveLdapEscape(rightValues)
				rightValues = rule.SanitizeValues(rightValues)

				if rule.LeftAttribute == "Roles" {
					if rule.RuleString != "" {
						rightValues = rule.FilterPreg(rule.RuleString, rightValues)
						rightValues = rule.FilterList(rule.SanitizeValues(strings.Split(rule.RuleString, ",")), rightValues)
					}
					rightValues = rule.AddPrefix(rule.RolePrefix, rightValues)
				}

				connector.SetAttribute(&ident, rule.LeftAttribute, rightValues)
			}
		}
	}
	return ident, nil
}
