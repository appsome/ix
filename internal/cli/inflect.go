package cli

import "strings"

// entityInflections derives the template variables the `entity` generator uses
// from a snake_case singular name (e.g. "charging_profile"):
//
//	Name             charging_profile   (snake singular — sql column refs)
//	NamePascal       ChargingProfile    (Go type / GraphQL type)
//	NameCamel        chargingProfile    (GraphQL field / json)
//	NamePlural       charging_profiles  (snake plural — table + REST path)
//	NamePluralPascal ChargingProfiles
//	NamePluralCamel  chargingProfiles   (GraphQL list field)
func entityInflections(name string) map[string]any {
	name = strings.ToLower(strings.TrimSpace(name))
	plural := pluralizeSnake(name)
	return map[string]any{
		"Name":             name,
		"NamePascal":       snakeToPascal(name),
		"NameCamel":        snakeToCamel(name),
		"NamePlural":       plural,
		"NamePluralPascal": snakeToPascal(plural),
		"NamePluralCamel":  snakeToCamel(plural),
	}
}

func snakeToPascal(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

func snakeToCamel(s string) string {
	p := snakeToPascal(s)
	if p == "" {
		return p
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// pluralizeSnake pluralizes the last underscore segment with simple English
// rules. Irregular plurals are the user's to fix in the generated files.
func pluralizeSnake(s string) string {
	parts := strings.Split(s, "_")
	last := parts[len(parts)-1]
	parts[len(parts)-1] = pluralizeWord(last)
	return strings.Join(parts, "_")
}

func pluralizeWord(w string) string {
	switch {
	case w == "":
		return w
	case strings.HasSuffix(w, "y") && !isVowel(w[len(w)-2]):
		return w[:len(w)-1] + "ies"
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"),
		strings.HasSuffix(w, "z"), strings.HasSuffix(w, "ch"),
		strings.HasSuffix(w, "sh"):
		return w + "es"
	default:
		return w + "s"
	}
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}
