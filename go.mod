module vmware-migration-kit

go 1.23.0

toolchain go1.23.2

require (
	github.com/gophercloud/gophercloud v1.14.1
	github.com/gophercloud/gophercloud/v2 v2.7.0
	github.com/sirupsen/logrus v1.9.3
	github.com/vmware/govmomi v0.50.0
	gopkg.in/yaml.v3 v3.0.1
	libguestfs.org/libnbd v1.20.0
)

require golang.org/x/sys v0.33.0 // indirect
