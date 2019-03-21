all: build run

watch-processes:
	watch -n1 'ps -eo pid,ppid,pgid,cmd | grep -E "(pm|script|zb)" | grep -Ev "(chrome|go|gvfs|grep|watch|acpi)"'

watch-processes-in-docker:
	watch -n1 "docker exec pm ps -elf | grep -v ps"

build:
	go build -o pm ../*.go
	docker build -t pm .

run:
	@ docker rm -f pm 2> /dev/null || echo -n
	docker run -d \
		--name pm \
		-p 8080:8080 \
		-e PROC_NAME=es \
		-e PROC_CMD="ex/script z m 2" \
		pm
	docker logs pm -f