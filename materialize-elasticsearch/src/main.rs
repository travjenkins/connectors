//mod schema;
//
//use doc::{
//    inference::{ArrayShape, ObjShape, Provenance, Shape},
//    SchemaIndex,
//};
//use elasticsearch::{http::transport::Transport, Elasticsearch, SearchParts};
//use serde_json::{json, Value};
//
//#[tokio::main]
//async fn main() -> Result<(), Box<dyn std::error::Error>> {
//    let transport = Transport::single_node("http://localhost:9200")?;
//    let client = Elasticsearch::new(transport);
//
//    // make a search API call
//    let search_response = client
//        .search(SearchParts::Index(&["jj-test-1"]))
//        .body(json!({
//            "query": {
//                "match_all": {}
//            }
//        }))
//        .allow_no_indices(true)
//        .send()
//        .await?;
//
//    // get the HTTP response status code
//    let status_code = search_response.status_code();
//
//    // read the response body. Consumes search_response
//    let response_body = search_response.json::<Value>().await?;
//
//    println!("{:#?}", response_body);
//
//    Ok(())
//}
