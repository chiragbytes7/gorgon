gorgon_couchbase \
    -gorgon-nodes $NODES \
    -gorgon-match '*~*~*' \
    -gorgon-concurrency 10 \
    -durability majority \
    -replicas 2 \
    run