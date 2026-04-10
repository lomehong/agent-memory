package core

import (
	"strings"

	"github.com/lomehong/agent-memory/internal/model"
)

// InferCategory determines the category of a memory based on its content.
// Corresponds to DESIGN-007.
func InferCategory(content string) string {
	lower := strings.ToLower(content)

	rules := map[string][]string{
		model.CategoryIdentity: {
			"我是", "我叫", "我的名字", "身份是", "角色", "人设",
			"userId", "时区", "邮箱", "github", "i am", "my name",
		},
		model.CategoryPrinciple: {
			"原则", "规则", "必须", "禁止", "不允许", "要求", "准则", "规范",
			"偏好", "习惯", "策略", "guideline", "policy", "principle",
			"优先于", "可靠性", "准确性",
		},
		model.CategoryKnowledge: {
			"技术栈", "架构", "框架", "服务器", "API", "端口",
			"团队", "Agent", "部署", "基础设施", "工具链",
			"technology", "framework", "architecture", "server",
		},
	}
	// Check in priority order: identity > principle > knowledge
	for _, category := range []string{model.CategoryIdentity, model.CategoryPrinciple, model.CategoryKnowledge} {
		keywords := rules[category]
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return category
			}
		}
	}

	return model.CategoryWorking
}

// InferVisibility determines the appropriate visibility for a memory.
// Corresponds to DESIGN-007-A.
//
// Rules:
//   - identity -> user (所有Agent共享用户身份信息)
//   - principle -> user (所有Agent应遵循的工作原则/偏好)
//   - knowledge -> team (项目规范/架构决策团队共享)
//   - working -> private (仅创建者可见的临时工作记忆)
func InferVisibility(content, category string) string {
	switch category {
	case model.CategoryIdentity:
		return model.VisibilityUser
	case model.CategoryPrinciple:
		return model.VisibilityUser
	case model.CategoryKnowledge:
		return model.VisibilityTeam
	case model.CategoryWorking:
		return model.VisibilityPrivate
	default:
		return model.VisibilityPrivate
	}
}

// InferPriority determines the default priority for a category.
// Corresponds to REQ-003: priority 1-5, 1=highest.
func InferPriority(category string) int {
	switch category {
	case model.CategoryIdentity:
		return 1
	case model.CategoryPrinciple:
		return 2
	case model.CategoryKnowledge:
		return 3
	case model.CategoryWorking:
		return 4
	default:
		return 3
	}
}

// InferTTL determines the default TTL for a category.
// Corresponds to REQ-008.
func InferTTL(category string) string {
	switch category {
	case model.CategoryIdentity:
		return model.TTLPermanent
	case model.CategoryPrinciple:
		return model.TTLPermanent
	case model.CategoryKnowledge:
		return model.TTLYear
	case model.CategoryWorking:
		return model.TTLMonth
	default:
		return model.TTLMonth
	}
}

// EvaluateContent evaluates content and returns a write suggestion.
// Corresponds to REQ-018, REQ-019.
func EvaluateContent(content string) (string, string, int, string) {
	category := InferCategory(content)
	visibility := InferVisibility(content, category)
	priority := InferPriority(category)
	ttl := InferTTL(category)
	return category, visibility, priority, ttl
}
