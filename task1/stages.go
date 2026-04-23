package main

import "log"

type Stage interface {
	Setup() error
	Process(msg *Message) error
	Teardown() error
}

type SchemaValidationStage struct{}

func (s *SchemaValidationStage) Setup() error {
	log.Println("SchemaValidationStage: setup")
	return nil
}

func (s *SchemaValidationStage) Process(msg *Message) error {
	log.Printf("SchemaValidationStage: processing message: %s", msg.Message)
	return nil
}

func (s *SchemaValidationStage) Teardown() error {
	log.Println("SchemaValidationStage: teardown")
	return nil
}

type FieldTransformationStage struct{}

func (s *FieldTransformationStage) Setup() error {
	log.Println("FieldTransformationStage: setup")
	return nil
}

func (s *FieldTransformationStage) Process(msg *Message) error {
	log.Printf("FieldTransformationStage: processing message: %s", msg.Message)
	return nil
}

func (s *FieldTransformationStage) Teardown() error {
	log.Println("FieldTransformationStage: teardown")
	return nil
}

type DeduplicationStage struct{}

func (s *DeduplicationStage) Setup() error {
	log.Println("DeduplicationStage: setup")
	return nil
}

func (s *DeduplicationStage) Process(msg *Message) error {
	log.Printf("DeduplicationStage: processing message: %s", msg.Message)
	return nil
}

func (s *DeduplicationStage) Teardown() error {
	log.Println("DeduplicationStage: teardown")
	return nil
}
