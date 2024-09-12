package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func getAllParameters(client *ssm.Client) ([]types.ParameterMetadata, error) {
	var parameters []types.ParameterMetadata
	input := &ssm.DescribeParametersInput{}
	paginator := ssm.NewDescribeParametersPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return nil, err
		}
		parameters = append(parameters, page.Parameters...)
		break
	}
	return parameters, nil
}

func getParameterDetails(client *ssm.Client, name string) (*types.Parameter, error) {
	fmt.Printf("Getting parameter details for: %v\n", name)
	input := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	}
	result, err := client.GetParameter(context.TODO(), input)
	if err != nil {
		return nil, err
	}
	return result.Parameter, nil
}

func connectToAWSByProfile(profile string) (*ssm.Client, error) {
	// Load the default configuration with the specified profile
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithSharedConfigProfile(profile))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %v", err)
	}

	client := ssm.NewFromConfig(cfg)
	return client, nil
}

// generateOldVariableName generates a variable name in the old format /asset-accounting/{environment}/{variableName}
func generateOldVariableName(environment, variableName string) string {
	return fmt.Sprintf("/asset-accounting/%s/%s", environment, variableName)
}

// generateNewVariableName generates a variable name in the new format /asset-accounting/serviceplatform/{environment}/{variableName}
func generateNewVariableName(environment, variableName string) string {
	return fmt.Sprintf("/asset-accounting/serviceplatform/%s/%s", environment, variableName)
}

// generateVariableNameMap generates a map from old variable names to new variable names
func generateVariableNameMap(environment string) map[string]string {
	//serverlessParams := []string{
	//	"REDISCLOUD_URL", "REDIS_ENABLED_TLS", "REDIS_DB", "LOG_LEVEL", "JAWSDB_URL",
	//	"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DB",
	//	"MYSQL_MAX_OPEN_CONNS", "MYSQL_MAX_IDLE_CONNS", "MYSQL_CONN_MAX_LIFETIME",
	//	"JAWSDB_REPLICATION_URL", "MYSQL_REPLICATION_HOST", "MYSQL_REPLICATION_PORT",
	//	"MYSQL_REPLICATION_USER", "MYSQL_REPLICATION_PASSWORD", "MYSQL_REPLICATION_DB",
	//	"MYSQL_REPLICATION_MAX_OPEN_CONNS", "MYSQL_REPLICATION_MAX_IDLE_CONNS",
	//	"MYSQL_REPLICATION_CONN_MAX_LIFETIME", "DD_API_KEY", "DD_SITE", "DD_ENV",
	//	"DD_SERVERLESS_LOGS_ENABLED", "DD_MERGE_XRAY_TRACES", "DD_TRACE_ENABLED",
	//	"ACCPLUS_BASE_URL",
	//}

	serverlessParams := []string{
		"REDISCLOUD_URL",
	}

	variableNameMap := make(map[string]string)
	for _, param := range serverlessParams {
		oldName := generateOldVariableName(environment, param)
		newName := generateNewVariableName(environment, param)
		variableNameMap[oldName] = newName
	}
	return variableNameMap
}

func getParameterDescription(client *ssm.Client, name string) (string, error) {
	input := &ssm.DescribeParametersInput{
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    aws.String("Name"),
				Values: []string{name},
			},
		},
	}
	output, err := client.DescribeParameters(context.TODO(), input)
	if err != nil {
		return "", err
	}
	if len(output.Parameters) == 0 {
		return "", fmt.Errorf("parameter not found")
	}
	return aws.ToString(output.Parameters[0].Description), nil
}

func putParameter(client *ssm.Client, name, description string, dest *types.Parameter) error {
	input := &ssm.PutParameterInput{
		Name:        aws.String(name),
		Value:       aws.String(*dest.Value),
		Type:        dest.Type,
		Description: aws.String(description),
	}
	_, err := client.PutParameter(context.TODO(), input)
	return err
}

func copyParameter(client *ssm.Client, sourceName, destName string) error {
	fmt.Printf(" =====================\n")
	sourceParam, err := getParameterDetails(client, sourceName)
	if err != nil {
		return fmt.Errorf("failed to get source parameter details: %v", err)
	}
	description, err := getParameterDescription(client, sourceName)
	if err != nil {
		return fmt.Errorf("failed to get source parameter description: %v", err)
	}
	fmt.Printf("name: %v, value: %v, type: %v, description: %v \n", *sourceParam.Name, *sourceParam.Value, *&sourceParam.Type, description)

	err = putParameter(client, destName, description, sourceParam)
	if err != nil {
		return fmt.Errorf("failed to put destination parameter: %v", err)
	}
	fmt.Printf("Success copied parameter from %v to %v\n", sourceName, destName)
	return nil
}

func main() {
	environemnt := "staging" // or "production" or beta
	profile := "aa_stg"

	if environemnt == "production" {
		profile = "aa_prod"
	}

	client, err := connectToAWSByProfile(profile)
	if err != nil {
		log.Fatalf("failed to connect to AWS, %v", err)
	}

	// Example usage
	//params, err := getAllParameters(client)
	//if err != nil {
	//	log.Fatalf("failed to get parameters, %v", err)
	//}

	oldToNewEnvName := generateVariableNameMap(environemnt)

	for oldEnvName, newEnName := range oldToNewEnvName {
		fmt.Printf("oldName: %v == newName: %v\n", oldEnvName, newEnName)
		// details, err := getParameterDetails(client, oldEnvName)
		// if err != nil {
		// 	log.Fatalf("failed to get parameter details, %v", err)
		// }
		// fmt.Printf("name: %v, value: %v, type: %v, description: %v \n", *details.Name, *details.Value, *&details.Type)

		err = copyParameter(client, oldEnvName, newEnName)
		if err != nil {
			log.Fatalf("failed to copy parameter, %v", err)
		}
	}
}
