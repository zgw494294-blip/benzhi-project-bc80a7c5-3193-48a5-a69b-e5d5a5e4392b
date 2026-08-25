package domain

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var digestPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func ValidateID(value, field string) error {
	if !identifierPattern.MatchString(value) {
		return Validation("invalid_"+field, field+" 必须由字母、数字、点、下划线或连字符组成，且长度不超过 64")
	}
	return nil
}

func requireText(value, field string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return Validation("missing_"+field, field+" 为必填项")
	}
	if len([]rune(value)) > max {
		return Validation("long_"+field, field+" 长度超出限制")
	}
	return nil
}

func ValidateBatchFields(title, venue, coordinator string, performanceAt time.Time) error {
	if err := requireText(title, "title", 120); err != nil {
		return err
	}
	if err := requireText(venue, "venue", 120); err != nil {
		return err
	}
	if err := requireText(coordinator, "coordinator", 80); err != nil {
		return err
	}
	if performanceAt.IsZero() {
		return Validation("missing_performance_at", "performanceAt 为必填项")
	}
	return nil
}

func ValidateUnit(unit SceneryUnit) error {
	if err := ValidateID(unit.UnitCode, "unitCode"); err != nil {
		return err
	}
	fields := []struct {
		value, name string
		max         int
	}{
		{unit.Name, "name", 120}, {unit.StageZone, "stageZone", 80},
		{unit.MaterialClass, "materialClass", 80}, {unit.Supplier, "supplier", 120},
		{unit.TreatmentLot, "treatmentLot", 80},
	}
	for _, field := range fields {
		if err := requireText(field.value, field.name, field.max); err != nil {
			return err
		}
	}
	if len(unit.EvidenceRefs) == 0 {
		return Validation("missing_evidence", "至少登记一项阻燃证据")
	}
	for _, evidence := range unit.EvidenceRefs {
		if err := requireText(evidence.Name, "evidence.name", 160); err != nil {
			return err
		}
		if err := requireText(evidence.Digest, "evidence.digest", 128); err != nil {
			return err
		}
		if !digestPattern.MatchString(strings.TrimSpace(evidence.Digest)) {
			return Validation("invalid_evidence_digest", "evidence.digest 格式无效")
		}
	}
	return nil
}

func ValidateEvidenceDigest(value, field string) error {
	if err := requireText(value, field, 128); err != nil {
		return err
	}
	if !digestPattern.MatchString(strings.TrimSpace(value)) {
		return Validation("invalid_"+strings.ReplaceAll(field, ".", "_"), field+" 格式无效")
	}
	return nil
}

func ValidateDefinitions(definitions []CheckDefinition) error {
	if len(definitions) == 0 {
		return Validation("empty_plan", "检查方案至少包含一个项目")
	}
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := ValidateID(definition.Code, "checkCode"); err != nil {
			return err
		}
		if _, exists := seen[definition.Code]; exists {
			return Conflict("duplicate_check_code", "检查项目编号不能重复")
		}
		seen[definition.Code] = struct{}{}
		if err := requireText(definition.Name, "check.name", 100); err != nil {
			return err
		}
		if err := requireText(definition.Criterion, "check.criterion", 240); err != nil {
			return err
		}
		if !definition.Required {
			return Validation("optional_check_not_supported", "方案中的检查项目必须标记为必检")
		}
	}
	return nil
}

func NormalizeDefinitions(input []CheckDefinition) []CheckDefinition {
	result := append([]CheckDefinition(nil), input...)
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func NormalizeEvidence(input []EvidenceRef) []EvidenceRef {
	result := append([]EvidenceRef(nil), input...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Digest == result[j].Digest {
			return result[i].Name < result[j].Name
		}
		return result[i].Digest < result[j].Digest
	})
	return result
}
