build:
	go build -o bin/releaser main.go

localTest:
	docker rm -f fake-gcs-server
	sleep 2
	docker run -d --name fake-gcs-server -p 4443:4443 --network helm fsouza/fake-gcs-server -external-url https://fake-gcs-server:4443 -public-host fake-gcs-server:4443
	sleep 2
	docker build . -t docker.elastic.co/eck-dev/go-helm-releaser:0.0.1 && docker run -it --network helm -v `pwd`/../../../deploy:/tmp/deploy -v /Users/mmontgomery/tmp/gcs-credentials.json:/tmp/credentials.json docker.elastic.co/eck-dev/go-helm-releaser:0.0.1 --charts-dir=/tmp/deploy --credentials-file=/tmp/credentials.json --google-gcs-url=https://fake-gcs-server:4443 --charts-repo-url=https://fake-gcs-server:4443 -u