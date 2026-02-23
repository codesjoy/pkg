// Package aipsqlgorm provides integration helpers between aipsql and GORM.
//
// It keeps aipsql-generated named parameters (for example @p_0) and converts
// them to sql.NamedArg so they can be used directly with gorm.DB query chains.
package aipsqlgorm
