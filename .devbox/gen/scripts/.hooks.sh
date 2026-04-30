echo 'Devbox-Umgebung für CodyMail (Go) geladen'
echo '-------------------------------------------'
go version
echo '-------------------------------------------'
echo -n 'golangci-lint: ' && golangci-lint --version
echo '-------------------------------------------'
echo -n 'Docker: ' && (docker info --format '{{.ServerVersion}}' 2>/dev/null || echo 'nicht erreichbar – Integrationstests werden fehlschlagen')
echo '-------------------------------------------'
echo 'Skripte: build | test | test-gateway | test-worker | test-admin | test-bounce | test-integration | lint | coverage | generate | up | down'