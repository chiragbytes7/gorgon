gorgon_couchbase \
    -gorgon-nodes $NODES \
    -gorgon-match '*~*~*' \
    -gorgon-concurrency 10 \
    -durability majorityPersistActive \
    -replicas 2 \
    run
