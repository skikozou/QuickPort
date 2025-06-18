package core

func (a *ShellArgs) Next() *ShellArgs {
	a.Arg = a.Arg[1:]
	return a
}

func (a *ShellArgs) Head() string {
	return a.Arg[0]
}

func (a *ShellArgs) Len() int {
	return len(a.Arg)
}
