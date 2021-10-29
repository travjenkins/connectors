use doc::inference::{ArrayShape, ObjShape, Shape};
use doc::Annotation;
use flow_json::schema::{self, build, index::IndexBuilder, types};
use json::JsonValue;

const UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES: &str =
    "multiple non-trivial data types or unspecified data types";
const UNSUPPORTED_NON_ARRAY_OR_OBJECTS: &str = "data types other than objects or arrays of objects";
const UNSUPPORTED_OBJECT_ADDITIONAL_FIELDS: &str = "additional properties on an object";
const UNSUPPORTED_TUPLE: &str = "Tuple is not supported";

#[derive(thiserror::Error, Debug)]
pub enum Error {
    #[error("parsing schema json")]
    SchemaJsonParsing(#[from] serde_json::Error),
    #[error("unsupported flow schema, details: {0}")]
    UnSupportedError(String),
}

pub fn build_elastic_schema(curi: url::Url, schema_json: &str) -> Result<JsonValue, Error> {
    let schema = match serde_json::from_str(schema_json) {
        Ok(v) => v,
        Err(e) => return Err(Error::SchemaJsonParsing(e)),
    };

    let schema = schema::build::build_schema::<Annotation>(curi, &schema).unwrap();
    let index = IndexBuilder::new().into_index();

    let shape = Shape::infer(&schema, &index);
    return build_from_shape(&shape);
}

fn err(error_message: &str) -> Result<JsonValue, Error> {
    Err(Error::UnSupportedError(error_message.to_string()))
}

fn build_from_shape(shape: &Shape) -> Result<JsonValue, Error> {
    let num_types = shape.type_.to_vec().len();
    if num_types > 2 || (num_types == 2 && !shape.type_.overlaps(types::NULL)) {
        return err(UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES);
    }

    // The shapes being processed are either of a single type (e.g. object),
    // or nullable multi-types(e.g. ["object", "null"]).
    if shape.type_.overlaps(types::OBJECT) {
        build_from_object(&shape.object)
    } else if shape.type_.overlaps(types::ARRAY) {
        build_from_array(&shape.array)
    } else {
        err(UNSUPPORTED_NON_ARRAY_OR_OBJECTS)
    }
}

fn build_field_from_shape(shape: &Shape) -> Result<JsonValue, Error> {
    let mut fields = Vec::new();

    if shape.type_.overlaps(types::OBJECT) {
        match build_from_object(&shape.object) {
            Ok(v) => fields.push(v),
            Err(e) => return Err(e),
        }
    }
    if shape.type_.overlaps(types::ARRAY) {
        match build_from_array(&shape.array) {
            Ok(v) => fields.push(v),
            Err(e) => return Err(e),
        }
    }
    if shape.type_.overlaps(types::BOOLEAN) {
        fields.push(JsonValue::String("boolean".to_string()));
    }
    if shape.type_.overlaps(types::FRACTIONAL) {
        fields.push(JsonValue::String("double".to_string()));
    } else if shape.type_.overlaps(types::INTEGER) {
        fields.push(JsonValue::String("long".to_string()));
    }
    if shape.type_.overlaps(types::STRING) {
        fields.push(JsonValue::String("keyword".to_string()));
    }

    if fields.is_empty() {
        Ok(JsonValue::Null)
    } else if fields.len() == 1 {
        Ok(fields.pop().unwrap())
    } else {
        return err(UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES);
    }
}

fn build_from_object(shape: &ObjShape) -> Result<JsonValue, Error> {
    if !shape.additional.is_none() {
        return err(UNSUPPORTED_OBJECT_ADDITIONAL_FIELDS);
    }

    let mut obj_schema = JsonValue::new_object();
    for prop in &shape.properties {
        match build_field_from_shape(&prop.shape) {
            Ok(v) => obj_schema[&prop.name] = v,
            Err(e) => return Err(e),
        }
    }

    return Ok(obj_schema);
}

fn build_from_array(shape: &ArrayShape) -> Result<JsonValue, Error> {
    if !shape.tuple.is_empty() {
        return err(UNSUPPORTED_TUPLE);
    }

    return match &shape.additional {
        None => err(UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES),
        // In Elastic search, the schema of an array is the same as the schema of its items.
        // https://www.elastic.co/guide/en/elasticsearch/reference/current/array.html
        Some(shape) => build_from_shape(shape),
    };
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_url() -> url::Url {
        url::Url::parse("http://test/dummy_schema").unwrap()
    }

    fn check_unsupported_error(actual_error: &Error, expected_error_message: &str) {
        assert!(matches!(actual_error, Error::UnSupportedError { .. }));
        if let Error::UnSupportedError(actual_error_message) = actual_error {
            assert_eq!(actual_error_message, expected_error_message)
        }
    }

    #[test]
    fn test_build_elastic_search_schema_with_error() {
        assert!(matches!(
            build_elastic_schema(test_url(), "A bad json schema").unwrap_err(),
            Error::SchemaJsonParsing { .. }
        ));

        let empty_schema_json = " { } ";
        check_unsupported_error(
            &build_elastic_schema(test_url(), empty_schema_json).unwrap_err(),
            UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES,
        );

        let multiple_types_schema_json = r#"{"type": ["integer", "string"]}"#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), multiple_types_schema_json).unwrap_err(),
            UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES,
        );

        let int_schema_json = r#"{"type": "integer"}"#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), int_schema_json).unwrap_err(),
            UNSUPPORTED_NON_ARRAY_OR_OBJECTS,
        );

        let multiple_field_types_schema_json = r#" { "type": "object", "properties": { "mul_type": {"type": ["boolean", "integer"] } } }"#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), multiple_field_types_schema_json).unwrap_err(),
            UNSUPPORTED_MULTIPLE_OR_UNSPECIFIED_TYPES,
        );

        let object_additional_field_schema_json = r#"
          {"type": "object", "additionalProperties": {"type": "integer"}, "properties": {"int": {"type": "integer"}}}
        "#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), object_additional_field_schema_json).unwrap_err(),
            UNSUPPORTED_OBJECT_ADDITIONAL_FIELDS,
        );

        let tuple_field_schema_json =
            r#"{"type": "array", "items": [{"type": "string"}, {"type": "integer"}]}"#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), tuple_field_schema_json).unwrap_err(),
            UNSUPPORTED_TUPLE,
        );

        let simple_array_schema_json = r#"{"type": "array", "items": {"type": "string"}}"#;
        check_unsupported_error(
            &build_elastic_schema(test_url(), simple_array_schema_json).unwrap_err(),
            UNSUPPORTED_NON_ARRAY_OR_OBJECTS,
        );
    }

    #[test]
    fn test_build_elastic_search_schema_all_types() {
        let schema_json = r#"
        {
            "$id":"file:///build.flow.yaml?ptr=/collections/a~1collection/schema",
            "properties":{
                "str": {"type": "string"},
                "str_or_null": {"type": ["string", "null"] },
                "int": {"type": "integer"},
                "int_or_null": {"type": ["integer", "null"] },
                "num": {"type": "number"},
                "num_or_null": {"type": ["number", "null"] },
                "bool": {"type": "boolean"},
                "bool_or_null": {"type": ["boolean", "null"]},
                "array": {"type": "array", "items": {"type": "object", "properties": {"arr_field": {"type": "string"}}}},
                "nested": {"type": "object", "required": [], "properties": {"nested_field": {"type": ["null", "integer"]}}}

            },
            "required":["str"],
            "type":"object"
        }
        "#;
        //print!(
        //    "{}",
        //    build_elastic_schema(test_url(), schema_json)
        //        .unwrap()
        //        .dump(),
        //);

        let actual = build_elastic_schema(test_url(), schema_json).unwrap();
        assert_eq!(
            actual,
            json::parse(
                r#"{
                "array": {"arr_field":"keyword"},
                "bool":"boolean",
                "bool_or_null":"boolean",
                "int":"long",
                "int_or_null":"long",
                "nested":{"nested_field":"long"},
                "num":"double",
                "num_or_null":"double",
                "str":"keyword",
                "str_or_null":"keyword"
            }"#
            )
            .unwrap()
        );
    }
}
