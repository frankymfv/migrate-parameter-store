package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type Flags struct {
	Environment            string
	AwsProfile             string
	IsCloneLambdaParameter bool
	IsGetAllParameter      bool
	PrefixGetAllParameter  string
}

// handleFlags handles the command line flags
func handleFlags() Flags {
	environment := flag.String("environment", "beta", "Environment to use (e.g., production, beta, staging)")
	awsProfile := flag.String("aws_profile", "aa_stg", "AWS awsProfile to use")
	isCloneLambdaParameter := flag.Bool("is_clone_lambda_parameter", false, "Clone lambda parameter")
	isGetAllParameter := flag.Bool("is_get_all_parameter", false, "Get all parameter")
	prefixGetAllParameter := flag.String("prefix_get_all_parameter", "/asset-accounting/", "Get all parameter with prefix")
	flag.Parse()

	*prefixGetAllParameter = fmt.Sprintf("%v%v", *prefixGetAllParameter, *environment)

	flags := Flags{Environment: *environment, AwsProfile: *awsProfile,
		IsCloneLambdaParameter: *isCloneLambdaParameter, IsGetAllParameter: *isGetAllParameter,
		PrefixGetAllParameter: *prefixGetAllParameter}
	fmt.Printf("flags: %+v\n", flags)
	return flags
}
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
	serverlessParams := []string{
		"REDISCLOUD_URL", "REDIS_ENABLED_TLS", "REDIS_DB", "LOG_LEVEL", "JAWSDB_URL",
		"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DB",
		"MYSQL_MAX_OPEN_CONNS", "MYSQL_MAX_IDLE_CONNS", "MYSQL_CONN_MAX_LIFETIME",
		"JAWSDB_REPLICATION_URL", "MYSQL_REPLICATION_HOST", "MYSQL_REPLICATION_PORT",
		"MYSQL_REPLICATION_USER", "MYSQL_REPLICATION_PASSWORD", "MYSQL_REPLICATION_DB",
		"MYSQL_REPLICATION_MAX_OPEN_CONNS", "MYSQL_REPLICATION_MAX_IDLE_CONNS",
		"MYSQL_REPLICATION_CONN_MAX_LIFETIME", "DD_API_KEY", "DD_SITE",
		"ERP_BASIC_AUTH_USER_NAME", "ERP_BASIC_AUTH_PASSWORD", "ERP_BASE_URL",
		"ERP_REQUEST_TIME_OUT", "CACHE_CONTRACT_EXPIRATION_TIME", "NAVIS_BASIC_AUTH_USER_NAME",
		"NAVIS_BASIC_AUTH_PASSWORD", "NAVIS_BASE_URL", "NAVIS_PROXY_URL", "DD_API_KEY",
		"DD_API_URL", "DD_SITE", "ROLLBAR_TOKEN", "NOTIFIER_ENGINE", "APP_ROOT_FILE_MANAGEMENT_SYSTEM",
		"S3_Bucket", "KMS_CMK_KEY_ID", "RECAL_LAMBDA_CONCURRENCY_MAX",
		"RECALC_YEARLY_CLOSING_BATCH_SIZE", "DATA_SCANNER_SLACK_CHANNEL", "DATA_SCANNER_WEBHOOK_URL",
		"DATA_SCANNER_SLACK_CHANNEL", "DATA_SCANNER_WEBHOOK_URL",
	}

	// serverlessParams := []string{
	// 	"REDISCLOUD_URL",
	// }

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
	fmt.Printf("start copy parameter sourceName: %v == destName: %v\n", sourceName, destName)
	sourceParam, err := getParameterDetails(client, sourceName)
	if err != nil {
		return fmt.Errorf("[FAILED] to get source parameter details: %v", err)
	}
	description, err := getParameterDescription(client, sourceName)
	if err != nil {
		return fmt.Errorf("[FAILED] to get source parameter description: %v", err)
	}
	fmt.Printf("name: %v, value: %v, type: %v, description: %v \n", *sourceParam.Name, *sourceParam.Value, *&sourceParam.Type, description)

	err = putParameter(client, destName, description, sourceParam)
	if err != nil {
		var parameterAlreadyExists *types.ParameterAlreadyExists
		if errors.As(err, &parameterAlreadyExists) {
			fmt.Printf("[DUPLICATED] Parameter %v already exists, skipping\n", destName)
			return nil
		}
		return fmt.Errorf("failed to put destination parameter: %v", err)
	}
	fmt.Printf("[SUCCESS] copied parameter from %v to %v\n", sourceName, destName)
	return nil
}

func cloneLambdaParameter(client *ssm.Client, flags Flags) error {
	oldToNewEnvName := generateVariableNameMap(flags.Environment)

	for oldEnvName, newEnName := range oldToNewEnvName {
		// fmt.Printf("oldName: %v == newName: %v\n", oldEnvName, newEnName)
		// details, err := getParameterDetails(client, oldEnvName)
		// if err != nil {
		// 	log.Fatalf("failed to get parameter details, %v", err)
		// }
		// fmt.Printf("name: %v, value: %v, type: %v, description: %v \n", *details.Name, *details.Value, *&details.Type)

		err := copyParameter(client, oldEnvName, newEnName)
		if err != nil {
			log.Fatalf("failed to copy parameter, %v", err)
		}
	}
	return nil
}

// saveDataToFile saves the given data to the specified file path as a JSON file.
func saveDataToFile(data map[string]interface{}, filePath string) {
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatalf("failed to create file, %v", err)
	}
	defer file.Close()

	encodedData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Fatalf("failed to encode data to JSON, %v", err)
	}

	if _, err := file.Write(encodedData); err != nil {
		log.Fatalf("failed to write data to file, %v", err)
	}
}

func main() {
	flags := handleFlags()

	client, err := connectToAWSByProfile(flags.AwsProfile)
	if err != nil {
		log.Fatalf("failed to connect to AWS, %v", err)
	}
	if flags.IsCloneLambdaParameter {
		fmt.Println("Start clone lambda parameter")
		err := cloneLambdaParameter(client, flags)
		if err != nil {
			log.Fatalf("failed to clone lambda parameter, %v", err)
		}
	}
	if flags.IsGetAllParameter {
		fmt.Println("Start get all parameter")
		savedDataList := map[string]interface{}{}
		params, err := getAllParameters(client)
		if err != nil {
			log.Fatalf("failed to get parameters, %v", err)
		}
		for _, param := range params {
			data := ""
			if param.Name == nil {
				data = fmt.Sprintf("%v, name: %v", data, param.Name)
			}
			if param.Type != "" {
				data = fmt.Sprintf("%v, type: %v", data, *&param.Type)
			}
			if param.Description != nil {
				data = fmt.Sprintf("%v, description: %v", data, *param.Description)
			}
			if strings.HasPrefix(*param.Name, flags.PrefixGetAllParameter) {
				fmt.Println(data)
				if paramDetail, err := getParameterDetails(client, *param.Name); err == nil {
					savedDataList[*param.Name] = paramDetail.Value
				} else {
					fmt.Errorf("failed to get source parameter details: %v", err)
				}
			} else {
				fmt.Printf("[NotInPrefix] prefix:%v == data: %v", flags.PrefixGetAllParameter, data)
			}
		}
		filePath := fmt.Sprintf("allParameters_%v.json", flags.Environment)
		saveDataToFile(savedDataList, filePath)
	}
}
