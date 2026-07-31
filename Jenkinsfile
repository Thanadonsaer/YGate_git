pipeline {
    agent { label 'ygate-linux-build' }

    parameters {
        string(name: 'PUBLIC_GATEWAY_URL', defaultValue: 'https://ygate.yokogawasolution.com', description: 'Public same-origin URL baked into the Web build')
        string(name: 'PRODUCTION_HOST', defaultValue: 'https://ygate.yokogawasolution.com', description: 'SSH host for production deployment')
    }

    environment {
        DEPLOY_CREDENTIALS = 'ygate-production-ssh'
        KNOWN_HOSTS_CREDENTIAL = 'ygate-production-known-hosts'
    }

    options {
        disableConcurrentBuilds()
        skipDefaultCheckout(true)
        timestamps()
        timeout(time: 45, unit: 'MINUTES')
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    stages {
        stage('Checkout') {
            steps {
                deleteDir()
                checkout scm
                script {
                    env.RELEASE_SHA = sh(script: 'git rev-parse HEAD', returnStdout: true).trim()
                }
            }
        }

        stage('Validate') {
            parallel {
                stage('Platform API') {
                    steps {
                        dir('services/platform-api') {
                            sh 'go test ./...'
                        }
                    }
                }
                stage('API Gateway') {
                    steps {
                        dir('services/api-gateway') {
                            sh 'go test ./...'
                        }
                    }
                }
                stage('Web') {
                    steps {
                        dir('apps/web') {
                            sh 'npm ci && npm run typecheck'
                        }
                    }
                }
            }
        }

        stage('Package') {
            when { branch 'main' }
            steps {
                sh '''
                    set -eu
                    mkdir -p release/bin release/web
                    (cd services/platform-api && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$RELEASE_SHA" -o ../../release/bin/platform-api ./cmd/platform-api)
                    (cd services/api-gateway && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ../../release/bin/api-gateway ./cmd/api-gateway)
                    (cd apps/web && NEXT_PUBLIC_GATEWAY_URL="$PUBLIC_GATEWAY_URL" npm run build)
                    cp -R apps/web/.next/standalone/. release/web/
                    cp packages/api-contracts/platform-api.yaml release/platform-api.yaml
                    printf '%s\n' "$RELEASE_SHA" > release/VERSION
                    tar -C release -czf "ygate-$RELEASE_SHA.tar.gz" .
                    sha256sum "ygate-$RELEASE_SHA.tar.gz" > "ygate-$RELEASE_SHA.tar.gz.sha256"
                '''
                archiveArtifacts artifacts: 'ygate-*.tar.gz,ygate-*.tar.gz.sha256', fingerprint: true
            }
        }

        stage('Approve Production') {
            when { branch 'main' }
            input {
                message "Deploy ${RELEASE_SHA} to production?"
                ok 'Deploy'
                submitter 'ygate-production-approvers'
            }
            steps {
                echo "Production deployment approved for ${RELEASE_SHA}"
            }
        }

        stage('Deploy Production') {
            when { branch 'main' }
            steps {
                withCredentials([
                    sshUserPrivateKey(credentialsId: env.DEPLOY_CREDENTIALS, keyFileVariable: 'SSH_KEY', usernameVariable: 'SSH_USER'),
                    file(credentialsId: env.KNOWN_HOSTS_CREDENTIAL, variable: 'KNOWN_HOSTS')
                ]) {
                    sh '''
                        set -eu
                        artifact="ygate-$RELEASE_SHA.tar.gz"
                        remote_artifact="/tmp/$artifact"
                        scp -i "$SSH_KEY" -o BatchMode=yes -o UserKnownHostsFile="$KNOWN_HOSTS" "$artifact" "$artifact.sha256" "$SSH_USER@$PRODUCTION_HOST:/tmp/"
                        ssh -i "$SSH_KEY" -o BatchMode=yes -o UserKnownHostsFile="$KNOWN_HOSTS" "$SSH_USER@$PRODUCTION_HOST" "sudo /usr/local/sbin/ygate-deploy '$remote_artifact' '$RELEASE_SHA'"
                    '''
                }
            }
        }
    }

    post {
        always {
            deleteDir()
        }
    }
}
