
run_get_all_param_beta:
	go run main.go -aws_profile aa_backend -environment beta -is_get_all_parameter true

run_get_all_param_stg:
	go run main.go -aws_profile aa_backend -environment staging -is_get_all_parameter true

run_get_all_param_prod:
	go run main.go -aws_profile aa_prod_2 -environment production -is_get_all_parameter true

run_lambda_param_prod:
	go run main.go -aws_profile aa_prod_2 -environment production -is_clone_lambda_parameter true

run_lambda_param_stg:
	go run main.go -aws_profile aa_backend -environment staging -is_clone_lambda_parameter true

run_lambda_param_beta:
	go run main.go -aws_profile aa_backend -environment beta -is_clone_lambda_parameter true
