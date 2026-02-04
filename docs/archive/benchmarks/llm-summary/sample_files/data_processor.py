"""
Data processing utilities for ETL pipelines.

This module provides classes and functions for transforming, validating,
and processing data in batch ETL workflows.
"""

import logging
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Callable, Dict, Generator, List, Optional, Protocol

import pandas as pd

logger = logging.getLogger(__name__)


@dataclass
class ProcessingConfig:
    """Configuration for data processing operations."""

    batch_size: int = 1000
    max_retries: int = 3
    timeout_seconds: int = 300
    validate_schema: bool = True
    drop_duplicates: bool = True
    fill_na_strategy: Optional[str] = None  # 'mean', 'median', 'mode', 'zero'


class DataValidator(Protocol):
    """Protocol for data validators."""

    def validate(self, df: pd.DataFrame) -> List[str]:
        """Validate dataframe and return list of errors."""
        ...


@dataclass
class ValidationResult:
    """Result of data validation."""

    is_valid: bool
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    rows_validated: int = 0
    timestamp: datetime = field(default_factory=datetime.utcnow)


class SchemaValidator:
    """Validates dataframe schema against expected columns and types."""

    def __init__(self, expected_schema: Dict[str, str]):
        self.expected_schema = expected_schema

    def validate(self, df: pd.DataFrame) -> List[str]:
        errors = []

        # Check for missing columns
        for col, dtype in self.expected_schema.items():
            if col not in df.columns:
                errors.append(f"Missing required column: {col}")
            elif str(df[col].dtype) != dtype:
                errors.append(
                    f"Column {col} has type {df[col].dtype}, expected {dtype}"
                )

        return errors


class NullValidator:
    """Validates that specified columns have no null values."""

    def __init__(self, non_null_columns: List[str]):
        self.non_null_columns = non_null_columns

    def validate(self, df: pd.DataFrame) -> List[str]:
        errors = []

        for col in self.non_null_columns:
            if col in df.columns:
                null_count = df[col].isna().sum()
                if null_count > 0:
                    errors.append(f"Column {col} has {null_count} null values")

        return errors


class DataProcessor:
    """
    Main data processing class for ETL operations.

    Handles transformation, validation, and batch processing of data.
    """

    def __init__(self, config: Optional[ProcessingConfig] = None):
        self.config = config or ProcessingConfig()
        self.validators: List[DataValidator] = []
        self.transforms: List[Callable[[pd.DataFrame], pd.DataFrame]] = []
        self._processing_stats: Dict[str, Any] = {}

    def add_validator(self, validator: DataValidator) -> "DataProcessor":
        """Add a validator to the processing pipeline."""
        self.validators.append(validator)
        return self

    def add_transform(
        self, transform: Callable[[pd.DataFrame], pd.DataFrame]
    ) -> "DataProcessor":
        """Add a transformation function to the pipeline."""
        self.transforms.append(transform)
        return self

    def validate(self, df: pd.DataFrame) -> ValidationResult:
        """Run all validators on the dataframe."""
        all_errors = []

        for validator in self.validators:
            errors = validator.validate(df)
            all_errors.extend(errors)

        return ValidationResult(
            is_valid=len(all_errors) == 0,
            errors=all_errors,
            rows_validated=len(df),
        )

    def transform(self, df: pd.DataFrame) -> pd.DataFrame:
        """Apply all transformations to the dataframe."""
        result = df.copy()

        for transform_fn in self.transforms:
            try:
                result = transform_fn(result)
            except Exception as e:
                logger.error(f"Transform failed: {e}")
                raise

        return result

    def process(self, df: pd.DataFrame) -> pd.DataFrame:
        """
        Process dataframe through the full pipeline.

        Steps:
        1. Validate input data
        2. Apply transformations
        3. Handle duplicates if configured
        4. Fill NA values if configured
        5. Final validation
        """
        start_time = datetime.utcnow()

        # Initial validation
        if self.config.validate_schema:
            result = self.validate(df)
            if not result.is_valid:
                raise ValueError(f"Validation failed: {result.errors}")

        # Apply transforms
        processed = self.transform(df)

        # Handle duplicates
        if self.config.drop_duplicates:
            before_count = len(processed)
            processed = processed.drop_duplicates()
            dropped = before_count - len(processed)
            if dropped > 0:
                logger.info(f"Dropped {dropped} duplicate rows")

        # Fill NA values
        if self.config.fill_na_strategy:
            processed = self._fill_na(processed)

        # Update stats
        self._processing_stats = {
            "input_rows": len(df),
            "output_rows": len(processed),
            "duration_seconds": (datetime.utcnow() - start_time).total_seconds(),
        }

        return processed

    def _fill_na(self, df: pd.DataFrame) -> pd.DataFrame:
        """Fill NA values based on configured strategy."""
        strategy = self.config.fill_na_strategy

        for col in df.select_dtypes(include=["number"]).columns:
            if strategy == "mean":
                df[col] = df[col].fillna(df[col].mean())
            elif strategy == "median":
                df[col] = df[col].fillna(df[col].median())
            elif strategy == "zero":
                df[col] = df[col].fillna(0)

        return df

    def process_batches(
        self, df: pd.DataFrame
    ) -> Generator[pd.DataFrame, None, None]:
        """Process dataframe in batches for memory efficiency."""
        batch_size = self.config.batch_size
        total_rows = len(df)

        for start_idx in range(0, total_rows, batch_size):
            end_idx = min(start_idx + batch_size, total_rows)
            batch = df.iloc[start_idx:end_idx].copy()

            logger.info(f"Processing batch {start_idx}-{end_idx} of {total_rows}")
            yield self.process(batch)

    @property
    def stats(self) -> Dict[str, Any]:
        """Get processing statistics."""
        return self._processing_stats.copy()
