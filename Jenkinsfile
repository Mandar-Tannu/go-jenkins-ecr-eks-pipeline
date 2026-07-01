pipeline {
    agent any

    environment {
        AWS_REGION = "ap-south-1"
        AWS_ACCOUNT_ID = "340541183493"
        ECR_REPOSITORY = "go-jenkins-ecr-eks-pipeline"
        IMAGE_NAME = "${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPOSITORY}"
        IMAGE_TAG = "${BUILD_NUMBER}"
        EKS_CLUSTER = "go-eks-cluster"
        DEPLOYMENT_NAME = "go-app"
        CONTAINER_NAME = "go-app"
        NAMESPACE = "default"
    }

    stages {

        stage('Checkout Source Code') {
            steps {
                git branch: 'master',
                    url: 'https://github.com/Mandar-Tannu/go-jenkins-ecr-eks-pipeline.git'
            }
        }

        stage('Build Go Application') {
            steps {
                sh 'go mod tidy'
                sh 'go build -o app .'
            }
        }

        stage('Run Unit Tests') {
            steps {
                sh 'go test ./...'
            }
        }

        stage('Build Docker Image') {
            steps {
                sh '''
                    docker build \
                    -t ${IMAGE_NAME}:${IMAGE_TAG} \
                    -t ${IMAGE_NAME}:latest .
                '''
            }
        }

        stage('Login to Amazon ECR') {
            steps {
                sh '''
                    aws ecr get-login-password --region ${AWS_REGION} | \
                    docker login \
                    --username AWS \
                    --password-stdin \
                    ${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com
                '''
            }
        }

        stage('Push Docker Image') {
            steps {
                sh '''
                    docker push ${IMAGE_NAME}:${IMAGE_TAG}
                    docker push ${IMAGE_NAME}:latest
                '''
            }
        }

        stage('Create/Update Kubernetes Secret') {
            steps {
                withCredentials([
                    usernamePassword(
                        credentialsId: 'rds-db-credentials',
                        usernameVariable: 'RDS_DB_USER',
                        passwordVariable: 'RDS_DB_PASSWORD'
                    )
                ]) {
                    sh '''
                        kubectl create secret generic go-app-secret \
                          --from-literal=RDS_DB_USER=$RDS_DB_USER \
                          --from-literal=RDS_DB_PASSWORD=$RDS_DB_PASSWORD \
                          --dry-run=client -o yaml | kubectl apply -f -
                    '''
                }
            }
        }

        stage('Deploy to Amazon EKS') {
            steps {
                sh '''
                    if kubectl get deployment ${DEPLOYMENT_NAME} -n ${NAMESPACE} > /dev/null 2>&1
                    then
                        echo "Deployment exists. Performing Rolling Update..."

                        kubectl set image deployment/${DEPLOYMENT_NAME} \
                        ${CONTAINER_NAME}=${IMAGE_NAME}:${IMAGE_TAG} \
                        -n ${NAMESPACE}

                    else
                        echo "First deployment..."

                        sed -i "s|IMAGE_PLACEHOLDER|${IMAGE_NAME}:${IMAGE_TAG}|g" deployment.yaml

                        kubectl apply -f go-app-configmap.yaml
                        kubectl apply -f deployment.yaml
                        kubectl apply -f service.yaml
                        kubectl apply -f ingress.yaml
                    fi
                '''
            }
        }

        stage('Verify Deployment') {
            steps {
                sh '''
                    kubectl rollout status deployment/${DEPLOYMENT_NAME} -n ${NAMESPACE}

                    kubectl get deployments

                    kubectl get pods

                    kubectl get service

                    kubectl get ingress
                '''
            }
        }

        stage('Cleanup') {
            steps {
                sh '''
                    docker image prune -f
                '''
            }
        }
    }

    post {

        success {
            echo 'Pipeline completed successfully.'
        }

        failure {
            echo 'Pipeline failed.'
        }

        always {
            cleanWs()
        }
    }
}
