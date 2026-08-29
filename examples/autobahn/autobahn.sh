podman run -it --rm \
    -v "${PWD}/config:/config" \
    -v "${PWD}/reports:/reports" \
    --network=host \
    --name fuzzingclient \
    crossbario/autobahn-testsuite:25.10.1 \
    wstest -m fuzzingclient -s /config/fuzzingclient.json