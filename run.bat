del ./*db
del ./*db.lock
start go run main.go N3
start go run main.go N2
start go run main.go N1
start go run main.go N0 len3.csv