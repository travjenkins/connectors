extern crate serde_with;

use std::fmt::Debug;
use std::io::Write;

pub mod airbyte;
pub mod catalog;
pub mod configuration;
pub mod connector;
pub mod halting;
pub mod kafka;
pub mod state;

pub struct KafkaConnector;

impl connector::Connector for KafkaConnector {
    type Config = configuration::Configuration;
    type ConfiguredCatalog = catalog::ConfiguredCatalog;
    type State = state::CheckpointSet;

    fn spec(output: &mut dyn Write) -> eyre::Result<()> {
        let message: airbyte::Spec<configuration::Configuration> = airbyte::Spec::new(true, vec![]);

        connector::write_message(output, message)?;
        Ok(())
    }

    fn check(output: &mut dyn Write, config: Self::Config) -> eyre::Result<()> {
        let consumer = kafka::consumer_from_config(&config)?;
        let message = kafka::test_connection(&consumer);

        connector::write_message(output, message)?;
        Ok(())
    }

    fn discover(output: &mut dyn Write, config: Self::Config) -> eyre::Result<()> {
        let consumer = kafka::consumer_from_config(&config)?;
        let metadata = kafka::fetch_metadata(&consumer)?;
        let streams = kafka::available_streams(&metadata);
        let message = airbyte::Catalog::new(streams);

        connector::write_message(output, message)?;
        Ok(())
    }

    fn read(
        output: &mut dyn Write,
        config: Self::Config,
        catalog: Self::ConfiguredCatalog,
        persisted_state: Option<Self::State>,
    ) -> eyre::Result<()> {
        let consumer = kafka::consumer_from_config(&config)?;
        let metadata = kafka::fetch_metadata(&consumer)?;

        let mut checkpoints = state::CheckpointSet::reconcile_catalog_state(
            &metadata,
            &catalog,
            &persisted_state.unwrap_or_default(),
        )?;
        kafka::subscribe(&consumer, &checkpoints)?;
        let watermarks = kafka::high_watermarks(&consumer, &checkpoints)?;
        let halt_check = halting::HaltCheck::new(&catalog, watermarks);

        while !halt_check.should_halt(&checkpoints) {
            let msg = consumer
                .poll(None)
                .expect("Polling without a timeout should always produce a message")
                .map_err(kafka::Error::Read)?;

            let (record, checkpoint) = kafka::process_message(&msg)?;

            let delta_state = airbyte::State::from(&checkpoint);
            checkpoints.add(checkpoint);

            connector::write_message(output, record)?;
            connector::write_message(output, delta_state)?;
        }

        Ok(())
    }
}

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("failed when interacting with kafka")]
    Kafka(#[from] kafka::Error),

    #[error("failed to process message")]
    Message(#[from] kafka::ProcessingError),

    #[error("failed to execute catalog")]
    Catalog(#[from] catalog::Error),

    #[error("failed to track state")]
    State(#[from] state::Error),
}
