module github.com/federico-pepe/push-tethered-app

go 1.26.3

require github.com/google/gousb v1.1.3

require github.com/federico-pepe/ableton-push-hack/core v0.0.0

require golang.org/x/image v0.41.0 // indirect

replace github.com/federico-pepe/ableton-push-hack/core => ../../Documents/GitHub/ableton-push-hack/core
