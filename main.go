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
	NewEnvName             string
	IsRemoveAllParameter   bool
}

// handleFlags handles the command line flags
func handleFlags() Flags {
	environment := flag.String("environment", "staging", "Environment to use (e.g., production, beta, staging)")
	newEnvName := flag.String("new_env_name", "stg", "Environment to use (e.g., production, beta, staging)")
	awsProfile := flag.String("aws_profile", "aa-be-stg", "AWS awsProfile to use")
	isCloneLambdaParameter := flag.Bool("is_clone_lambda_parameter", false, "Clone lambda parameter")
	isGetAllParameter := flag.Bool("is_get_all_parameter", false, "Get all parameter")
	prefixGetAllParameter := flag.String("prefix_get_all_parameter", "/asset-accounting", "Get all parameter with prefix")
	isRemoveAllParameter := flag.Bool("is_remove_all_parameter", false, "Remove all parameter")
	flag.Parse()

	*prefixGetAllParameter = fmt.Sprintf("%v/serviceplatform/%v", *prefixGetAllParameter, *environment)

	flags := Flags{Environment: *environment, AwsProfile: *awsProfile,
		IsCloneLambdaParameter: *isCloneLambdaParameter, IsGetAllParameter: *isGetAllParameter,
		PrefixGetAllParameter: *prefixGetAllParameter, NewEnvName: *newEnvName, IsRemoveAllParameter: *isRemoveAllParameter}
	fmt.Printf("flags: %+v\n", flags)
	return flags
}
func getAllParameters(client *ssm.Client, targetEnv string) ([]types.ParameterMetadata, error) {
	var parameters []types.ParameterMetadata
	input := &ssm.DescribeParametersInput{
		Filters: []types.ParametersFilter{
			{
				Key:    types.ParametersFilterKeyName,
				Values: []string{fmt.Sprintf("/asset-accounting/serviceplatform/%v/", targetEnv)},
			},
		},
	}
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

func copyParameter(client *ssm.Client, sourceParam types.ParameterMetadata, destName string) error {
	fmt.Printf(" =====================\n")
	fmt.Printf("start copy parameter sourceName: %v == destName: %v\n", *sourceParam.Name, destName)
	sourceData, err := getParameterDetails(client, *sourceParam.Name)
	if err != nil {
		return fmt.Errorf("[FAILED] to get source parameter details: %v", err)
	}

	description := ""
	if sourceParam.Description != nil {
		description = *sourceParam.Description
	}

	fmt.Printf("name: %v, value: %v, type: %v, description: %v \n", *sourceParam.Name, *sourceData.Value, *&sourceParam.Type, description)

	err = putParameter(client, destName, description, sourceData)
	if err != nil {
		var parameterAlreadyExists *types.ParameterAlreadyExists
		if errors.As(err, &parameterAlreadyExists) {
			fmt.Printf("[DUPLICATED] Parameter %v already exists, skipping\n", destName)
			return nil
		}
		return fmt.Errorf("failed to put destination parameter: %v", err)
	}
	fmt.Printf("[SUCCESS] copied parameter from %v to %v\n", *sourceParam.Name, destName)
	return nil
}

func generateNewVariableNameForEnvStandard(sourceParamName, environment, newEnvName string) string {
	// e.g., /asset-accounting/serviceplatform/staging/REDIS_URL => /asset-accounting/serviceplatform/stg/REDIS_URL
	// e.g., /asset-accounting/serviceplatform/production/REDIS_URL => /asset-accounting/serviceplatform/prod/REDIS_URL
	return strings.Replace(sourceParamName, environment, newEnvName, -1)
}

func convertEnvNameToEnvStandardNameOfParameter(client *ssm.Client, flags Flags) error {
	params, err := getAllParameters(client, flags.Environment)
	if err != nil {
		log.Fatalf("failed to get parameters, %v", err)
	}
	for _, param := range params {
		if param.Name == nil {
			continue
		}
		newParamName := generateNewVariableNameForEnvStandard(*param.Name, flags.Environment, flags.NewEnvName)
		err := copyParameter(client, param, newParamName)
		if err != nil {
			log.Fatalf("failed to copy parameter, %v", err)
			return err
		}
	}
	return nil
}

func removeAllParameters(client *ssm.Client, targetEnv string) error {
	params, err := getAllParameters(client, targetEnv)
	if err != nil {
		return err
	}
	for _, param := range params {
		if param.Name == nil {
			continue
		}
		fmt.Println("Removing parameter: ", *param.Name)
		err := removeParameter(client, *param.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func removeParameter(client *ssm.Client, name string) error {
	input := &ssm.DeleteParameterInput{
		Name: aws.String(name),
	}
	_, err := client.DeleteParameter(context.TODO(), input)
	return err
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
	if flags.IsRemoveAllParameter {
		fmt.Println("Start remove all parameter")
		err := removeAllParameters(client, flags.Environment)
		if err != nil {
			log.Fatalf("failed to remove all parameters, %v", err)
			return
		}
	}

	if flags.IsCloneLambdaParameter {
		fmt.Println("Start clone lambda parameter")
		// err := cloneLambdaParameter(client, flags)
		err := convertEnvNameToEnvStandardNameOfParameter(client, flags)
		if err != nil {
			log.Fatalf("failed to clone lambda parameter, %v", err)
		}
	}
	if flags.IsGetAllParameter {
		fmt.Println("Start get all parameter")
		savedDataList := map[string]interface{}{}
		params, err := getAllParameters(client, flags.Environment)
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
				fmt.Printf("[NotInPrefix] prefix:%v == data: %v\n\n", flags.PrefixGetAllParameter, data)
			}
		}
		filePath := fmt.Sprintf("allParameters_%v.json", flags.Environment)
		saveDataToFile(savedDataList, filePath)
	}
}
