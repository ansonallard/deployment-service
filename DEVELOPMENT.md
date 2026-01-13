## Pulling Go packages from artifact store

To pull generated packages from the artifact store, you need to configure the following vars:

GONOSUMDB=<hostname> \
GOPROXY=https://<username>:<PAT>@<hostname>/api/packages/<team>/go,https://proxy.golang.org,direct \
go get -x <hostname>/<team>/<package>>@<version>