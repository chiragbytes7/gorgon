pipeline {
    agent any
    environment {
        PATH = "/usr/local/bin:/opt/homebrew/bin:${env.PATH}"
    }
    stages {
        stage('Make') {
            steps {
                dir('docker/gorgon_couchbase') {
                    sh 'make'
                }
            }
        }
        stage('Build Images') {
            steps {
                dir('docker/gorgon_couchbase') {
                    sh 'make build'
                }
            }
        }
        stage('Run Tests') {
            steps {
                dir('docker/gorgon_couchbase') {
                    sh './up.sh'
                }
            }
        }
        stage('Collect Results') {
            steps {
                sh 'docker cp gorgon-control:/files.tgz .'
                archiveArtifacts artifacts: 'files.tgz'
            }
        }
    }
    post {
        always {
            dir('docker/gorgon_couchbase') {
                sh 'docker compose -f compose.yaml down'
            }
        }
    }
}
