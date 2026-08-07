pipeline {
    agent any

    parameters {
        string(name: 'PUBLIC_GATEWAY_URL', defaultValue: 'https://ygate.yokogawasolution.com', description: 'Public same-origin URL baked into the Web build')
        string(name: 'RELEASES_ROOT', defaultValue: 'C:\\YGate\\releases', description: 'Directory on this machine where each release is unpacked')
        string(name: 'ENV_FILE', defaultValue: 'C:\\YGate\\ygate.env', description: 'Shared production environment file (not touched by deploys)')
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
                    env.RELEASE_SHA = powershell(script: 'git rev-parse HEAD', returnStdout: true).trim()
                    if (!env.BRANCH_NAME) {
                        env.BRANCH_NAME = powershell(script: 'git rev-parse --abbrev-ref HEAD', returnStdout: true).trim()
                    }
                }
            }
        }

        stage('Validate') {
            parallel {
                stage('Platform API') {
                    steps {
                        dir('services/platform-api') {
                            powershell '''
                                $ErrorActionPreference = "Stop"
                                go test ./...
                                if ($LASTEXITCODE -ne 0) { exit 1 }
                            '''
                        }
                    }
                }
                stage('API Gateway') {
                    steps {
                        dir('services/api-gateway') {
                            powershell '''
                                $ErrorActionPreference = "Stop"
                                go test ./...
                                if ($LASTEXITCODE -ne 0) { exit 1 }
                            '''
                        }
                    }
                }
                stage('Auth Service') {
                    steps {
                        dir('services/auth-service') {
                            powershell '''
                                $ErrorActionPreference = "Stop"
                                go test ./...
                                if ($LASTEXITCODE -ne 0) { exit 1 }
                            '''
                        }
                    }
                }
                stage('Web') {
                    steps {
                        dir('apps/web') {
                            powershell '''
                                $ErrorActionPreference = "Stop"
                                npm ci
                                if ($LASTEXITCODE -ne 0) { exit 1 }
                                npm run typecheck
                                if ($LASTEXITCODE -ne 0) { exit 1 }
                            '''
                        }
                    }
                }
            }
        }

        stage('Package') {
            steps {
                powershell '''
                    $ErrorActionPreference = "Stop"
                    $env:YGATE_RELEASE_VERSION = "jenkins-$env:BUILD_NUMBER-$($env:RELEASE_SHA.Substring(0, 12))"
                    .\\deploy\\manual\\build-release.ps1 -OutputDirectory dist\\jenkins -PublicGatewayUrl $env:PUBLIC_GATEWAY_URL -ReleaseVersion $env:YGATE_RELEASE_VERSION
                '''
                archiveArtifacts artifacts: 'dist/jenkins/ygate-*.zip,dist/jenkins/ygate-*.zip.sha256', fingerprint: true
            }
        }

        stage('Approve Production') {
            input {
                message "Deploy ${env.RELEASE_SHA} to production?"
                ok 'Deploy'
                submitter 'ygate-production-approvers'
            }
            steps {
                echo "Production deployment approved for ${env.RELEASE_SHA}"
            }
        }

        stage('Deploy Production') {
            steps {
                powershell '''
                    $ErrorActionPreference = "Stop"
                    $zip = Get-ChildItem "dist\\jenkins\\ygate-*.zip" | Select-Object -First 1
                    if (-not $zip) { throw "No release artifact found in dist\\jenkins" }
                    $release = $zip.BaseName -replace '^ygate-', ''
                    $target = Join-Path $env:RELEASES_ROOT $release
                    if (Test-Path $target) { throw "Release $release already exists at $target" }
                    Write-Host "Deploying $release to $target (env file: $env:ENV_FILE)"
                    Expand-Archive -Path $zip.FullName -DestinationPath $target
                    Push-Location $target
                    try {
                        & .\\start.ps1 -EnvFile $env:ENV_FILE
                        if ($LASTEXITCODE -ne 0) { throw "start.ps1 failed" }
                    } finally {
                        Pop-Location
                    }
                '''
            }
        }
    }

    post {
        always {
            deleteDir()
        }
    }
}
