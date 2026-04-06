set -o errexit
set -o pipefail
set -o nounset
shopt -s nullglob

export PATH=/src/gorgon_couchbase:$PATH

wait_for_node() {
    until nc -q 1 "$1" "$2" < /dev/null ; do sleep 1 ; done
}

echo "Nodes: $NODES"
for node in ${NODES//,/ } ; do
    wait_for_node $node 9090
done

touch /root/store/gorgon_json.log

{
    for workload in /workloads/*.sh ; do
        echo
        echo "Running $workload"
        echo
        bash "$workload" || break
    done
} 2>&1 | tee gorgon.log

tar -czf files.tgz gorgon.log *.html

echo
echo DONE
