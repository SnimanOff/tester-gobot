package launcher

type ChildUser struct {
	UID int
	GID int
}

func (childUser ChildUser) Enabled() bool {
	return childUser.UID >= 0
}

func (childUser ChildUser) groupID() int {
	if childUser.GID >= 0 {
		return childUser.GID
	}
	return childUser.UID
}
