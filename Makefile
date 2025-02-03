
run_get_all_param_beta:
	go run main.go -aws_profile aa-be-stg -environment beta -is_get_all_parameter false

run_get_all_param_stg:
	go run main.go -aws_profile aa-be-stg -environment staging -is_get_all_parameter false

run_lambda_param_stg:
	go run main.go -aws_profile aa-be-stg a_backend -environment staging -is_clone_lambda_parameter false

run_lambda_param_beta:
	go run main.go -aws_profile aa-be-stg -environment beta -is_clone_lambda_parameter false

run_get_all_param_prod:
	go run main.go -aws_profile aa_prod_2 -environment production -is_get_all_parameter false

run_lambda_param_prod:
	go run main.go -aws_profile aa_prod_2 -environment production -is_clone_lambda_parameter false


run_STG_migrate_env_standard:
	go run main.go -aws_profile aa-be-stg -environment staging -new_env_name stg -is_clone_lambda_parameter false

run_PROD_migrate_env_standard:
	go run main.go -aws_profile aa_prod -environment production -new_env_name prod -is_clone_lambda_parameter false


