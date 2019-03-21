all: build run

build:
	go build
	docker build -t pm .

run:
	@ docker rm -f pm 2> /dev/null || echo -n
	docker run -d \
		--name pm \
		-p 8081:8080 \
		-e PM_PROC_NAME=es \
		-e PM_PROC_CMD="bin/script zombies trap recursive" \
		pm
	docker logs pm -f

tests: build
	@ ./tests.sh

dev:
	PM_PROC_NAME=es \
	PM_PROC_CMD="../../pkg/controller/elasticsearch/processmanager/fixtures/script zombies trap recursive" \
	go run main.go

watch-processes:
	watch -n1 'ps -eo pid,ppid,pgid,cmd | grep -E "(pm|script|zb)" | grep -Ev "(chrome|go|gvfs|grep|watch|acpi)"'

watch-processes-in-docker:
	watch -n1 "docker exec pm ps -elf | grep -v ps"
