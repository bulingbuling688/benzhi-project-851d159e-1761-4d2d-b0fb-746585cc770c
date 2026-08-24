package application

type ActorRole string

const (
	RoleEngineer       ActorRole = "data_engineer"
	RoleReviewer       ActorRole = "sensitive_reviewer"
	RoleReleaseManager ActorRole = "release_manager"
)

func authorize(actual ActorRole, allowed ...ActorRole) error {
	for _, role := range allowed {
		if actual == role {
			return nil
		}
	}
	return fail(KindUnauthorized, "role_forbidden", "角色 %s 无权执行此操作", actual)
}

type Actor struct {
	Name string
	Role ActorRole
}

func (a Actor) Validate() error {
	if a.Name == "" {
		return fail(KindValidation, "missing_actor", "Actor-Name 不能为空")
	}
	if a.Role != RoleEngineer && a.Role != RoleReviewer && a.Role != RoleReleaseManager {
		return fail(KindUnauthorized, "invalid_role", "Actor-Role 无效")
	}
	return nil
}
