package handlers

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestIsSystemADGroup(t *testing.T) {
	systemCases := []string{
		"Domain Users",
		"domain users",
		"Administrators",
		"Domain Admins",
		"Enterprise Admins",
		"Users",
		"Guests",
		"Account Operators",
		"Cert Publishers",
		"DnsAdmins",
	}

	for _, g := range systemCases {
		if !isSystemADGroup(g) {
			t.Errorf("expected isSystemADGroup(%q) to be true", g)
		}
	}

	customCases := []string{
		"Engineering",
		"Developers",
		"Marketing Team",
		"Sales",
		"DevOps",
		"Security Operations",
		"Finance",
	}

	for _, g := range customCases {
		if isSystemADGroup(g) {
			t.Errorf("expected isSystemADGroup(%q) to be false", g)
		}
	}
}

func TestExtractADTeams(t *testing.T) {
	entry := &ldap.Entry{
		DN: "CN=Alice Smith,OU=Engineering,DC=company,DC=local",
		Attributes: []*ldap.EntryAttribute{
			{
				Name:   "department",
				Values: []string{"Engineering"},
			},
			{
				Name: "memberOf",
				Values: []string{
					"CN=Domain Users,CN=Users,DC=company,DC=local",
					"CN=Frontend Developers,OU=Security Groups,DC=company,DC=local",
					"CN=DevOps Team,OU=Security Groups,DC=company,DC=local",
					"CN=Administrators,CN=Builtin,DC=company,DC=local",
				},
			},
		},
	}

	teams := extractADTeams(entry)

	expected := map[string]bool{
		"Engineering":         true,
		"Frontend Developers": true,
		"DevOps Team":         true,
	}

	if len(teams) != len(expected) {
		t.Fatalf("expected %d teams, got %d: %v", len(expected), len(teams), teams)
	}

	for _, team := range teams {
		if !expected[team] {
			t.Errorf("unexpected team extracted: %s", team)
		}
	}
}
