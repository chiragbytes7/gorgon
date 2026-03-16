gorgon_couchbase \
    -gorgon-nodes $NODES \
    -gorgon-match '*~*~*' \
    -gorgon-concurrency 18 \
    -durability majorityPersistActive \
    -replicas 2 \
    -client-over-rpc \
    run
