set -o errexit
set -o pipefail
set -o nounset
shopt -s nullglob

export PATH=/src/gorgon_couchbase:$PATH

wait_for_node() {
    until nc -q 1 "$1" "$2" < /dev/null ; do sleep 1 ; done
}

wait_for_node n0.local 9090
wait_for_node n1.local 9090
wait_for_node n2.local 9090

export NODES=${NODES:-'n0.local,n1.local,n2.local'}

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
