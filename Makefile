build:
	cd auth-service && go build -o build/main
	cd ..
	cd http-gateway && go build -o build/main
	cd ..
	docker-compose up -d --build
