package version

var version = "dev"

func String() string {
	return version
}

func SetForTest(v string) func() {
	original := version
	version = v

	return func() { version = original }
}
