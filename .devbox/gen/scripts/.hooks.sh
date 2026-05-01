echo 'Devbox-Umgebung für Dispatch (Go) geladen'
echo '-------------------------------------------'
go version
echo '-------------------------------------------'
echo -n 'golangci-lint: ' && golangci-lint --version
echo '-------------------------------------------'
echo -n 'Docker: ' && (docker info --format '{{.ServerVersion}}' 2>/dev/null || echo 'nicht erreichbar – Integrationstests werden fehlschlagen')
echo '-------------------------------------------'
echo 'Skripte: build | test | test-* | lint | coverage | coverage-html | mutate | metrics | sonar | generate | up | down | up-proxy | down-proxy | run-worker-dev | run-gateway-dev'