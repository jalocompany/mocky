package services

import (
	"common/domain/customctx"
	"common/domain/logger"
	"common/utils"
	"common/utils/cerrs"
	"encoding/json"
	"fmt"
	"io"
	"mocky/internal/context/controllers/placeholder"
	"mocky/internal/core/settings"
	"net/http"
	"regexp"
	"strings"
	"time"
)

func (s *PrototypesService) Mock(cc *customctx.CustomContext, request *http.Request, pathParams map[string]string, headers map[string]string, query map[string]string) utils.Response[map[string]any] {

	entry := logger.FromContext(cc.Context())

	entry.Info("Mocking request")

	realPath := strings.TrimPrefix(request.URL.Path, "/"+settings.Settings.ROOT_PATH+"/v1/mocky")

	prototypeModel := s.prototypesRepository.GetByPath(cc, realPath, request.Method)

	if prototypeModel.Err != nil {
		entry.Error(prototypeModel.Err.Error())
		return utils.Response[map[string]any]{
			Error:      prototypeModel.Err,
			StatusCode: http.StatusNotFound,
			Success:    false,
		}
	}

	// Verificar los Headers
	headersResult := s.verifyHeaders(cc, prototypeModel.Data.ID, request, prototypeModel.Data.Request.Headers)
	if headersResult.Err != nil {
		entry.Error(headersResult.Err.Error())
		return utils.Response[map[string]any]{
			Error:      headersResult.Err,
			StatusCode: http.StatusBadRequest,
			Success:    false,
		}
	}

	// verificar los path params
	pathParamsResult := s.verifyPathParams(cc, prototypeModel.Data.ID, request, prototypeModel.Data.Request.PathParams)
	if pathParamsResult.Err != nil {
		entry.Error(pathParamsResult.Err.Error())
		return utils.Response[map[string]any]{
			Error:      pathParamsResult.Err,
			StatusCode: http.StatusBadRequest,
			Success:    false,
		}
	}

	// Verificar las Properties de la request

	bodyMap, err := _convertBodyToMap(request)
	if err != nil {
		entry.Error(err.Error())
		return utils.Response[map[string]any]{
			Error:      cerrs.NewCustomError(http.StatusInternalServerError, err.Error(), "convert_body_to_map"),
			StatusCode: http.StatusInternalServerError,
			Success:    false,
		}
	}

	if prototypeModel.Data.Request.BodySchema != nil && prototypeModel.Data.Request.BodySchema.TypeSchema != "" {

		propertiesResult := s.validator.Validate(*prototypeModel.Data.Request.BodySchema, bodyMap)
		if len(propertiesResult) > 0 {

			for _, err := range propertiesResult {
				entry.Error(err.String())
				cc.NewError(cerrs.NewCustomError(http.StatusUnprocessableEntity, err.String(), "validate_body"))
			}

			return utils.Response[map[string]any]{
				Error:      cerrs.NewCustomError(http.StatusUnprocessableEntity, propertiesResult[len(propertiesResult)-1].String(), "validate_body"),
				StatusCode: http.StatusUnprocessableEntity,
				Success:    false,
			}
		}

	}
	// Contruir la respuesta

	pathParamsMap := request.URL.Query()
	headersMap := request.Header

	fmt.Println(pathParamsMap)
	fmt.Println(headersMap)
	fmt.Println(bodyMap)

	mockContext := placeholder.MockContext{
		PathParams: pathParams,
		Query:      query,
		Headers:    headers,
		Body:       bodyMap,
	}

	resolved, err := s.placeholderController.Resolve(mockContext, prototypeModel.Data.Response.Body)
	if err != nil {
		entry.Error(err.Error())
		return utils.Response[map[string]any]{
			Error:      cerrs.NewCustomError(http.StatusInternalServerError, err.Error(), "placeholder_controller"),
			StatusCode: http.StatusInternalServerError,
			Success:    false,
		}
	}

	pretty, _ := json.MarshalIndent(resolved, "", "  ")
	fmt.Println("=== Response con valores (faker + args opcionales) ===")
	fmt.Println(string(pretty))

	if prototypeModel.Data.Request.Delay > 0 {
		time.Sleep(time.Duration(prototypeModel.Data.Request.Delay) * time.Millisecond)
	}

	entry.Infof("PrototypeModel: %v", prototypeModel.Data)

	return utils.Response[map[string]any]{
		Data:       resolved.(map[string]any),
		StatusCode: http.StatusOK,
	}
}

func (s *PrototypesService) verifyPathParams(
	cc *customctx.CustomContext,
	prototypeID string,
	request *http.Request,
	pathParamsSchemas map[string]string,
) utils.Result[map[string]interface{}] {
	entry := logger.FromContext(cc.Context())

	entry.Info("Verifying headers")

	for pathParam, schema := range pathParamsSchemas {
		fmt.Println(pathParam, schema)

		pathParamReceived := request.URL.Query().Get(pathParam)
		if pathParamReceived == "" {
			return utils.Result[map[string]interface{}]{Err: cerrs.NewCustomError(http.StatusBadRequest, "Path param "+pathParam+" is required", "verify_path_params")}
		}

		if strings.HasPrefix(schema, "^") {
			regex := schema[1:]
			match, _ := regexp.MatchString(regex, pathParamReceived)
			if !match {
				return utils.Result[map[string]interface{}]{Err: cerrs.NewCustomError(http.StatusBadRequest, "Path param "+pathParam+" does not match the schema", "verify_path_params")}
			}
		} else {
			if pathParamReceived != schema {
				return utils.Result[map[string]interface{}]{
					Err: cerrs.NewCustomError(
						http.StatusBadRequest,
						"Path param "+pathParam+" does not match the schema, check the prototype with ID: "+prototypeID,
						"verify_path_params",
					),
				}
			}
		}
	}

	return utils.Result[map[string]interface{}]{Data: map[string]interface{}{}}
}

func (s *PrototypesService) verifyHeaders(
	cc *customctx.CustomContext,
	prototypeID string,
	request *http.Request,
	headersSchemas map[string]string,
) utils.Result[map[string]interface{}] {

	entry := logger.FromContext(cc.Context())

	entry.Info("Verifying headers")

	for header, schema := range headersSchemas {
		fmt.Println(header, schema)

		headerReceived := request.Header.Get(header)
		if headerReceived == "" {
			return utils.Result[map[string]interface{}]{Err: cerrs.NewCustomError(http.StatusBadRequest, "Header "+header+" is required", "verify_headers")}
		}

		if strings.HasPrefix(schema, "^") {
			regex := schema[1:]
			match, _ := regexp.MatchString(regex, headerReceived)
			if !match {
				return utils.Result[map[string]interface{}]{Err: cerrs.NewCustomError(http.StatusBadRequest, "Header "+header+" does not match the schema", "verify_headers")}
			}
		} else {
			if headerReceived != schema {
				return utils.Result[map[string]interface{}]{
					Err: cerrs.NewCustomError(
						http.StatusBadRequest,
						"Header "+header+" does not match the schema, check the prototype with ID: "+prototypeID,
						"verify_headers",
					),
				}
			}
		}
	}

	return utils.Result[map[string]interface{}]{Data: map[string]interface{}{}}
}

func _convertBodyToMap(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if len(bodyBytes) == 0 {
		return map[string]any{}, nil
	}

	var bodyMap map[string]any
	if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
		// retornamos directamente el JSON como map[string]any
		return bodyMap, nil
	}

	// si no es JSON válido, lo devolvemos como {"raw": "..."}
	return map[string]any{"raw": string(bodyBytes)}, nil
}
