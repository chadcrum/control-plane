package controller

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	catalogv1alpha1 "github.com/dcm-project/control-plane/api/catalog/v1alpha1"
	catalogservice "github.com/dcm-project/control-plane/internal/catalog/service"
)

func boolPtr(v bool) *bool { return &v }

func catalogItemWithFields(fields []catalogv1alpha1.FieldConfiguration) *catalogv1alpha1.CatalogItem {
	return catalogItemWithResources([]catalogv1alpha1.CatalogResource{{
		Name:   "backend",
		Fields: &fields,
	}})
}

func catalogItemWithResources(resources []catalogv1alpha1.CatalogResource) *catalogv1alpha1.CatalogItem {
	return &catalogv1alpha1.CatalogItem{
		Spec: &catalogv1alpha1.CatalogItemSpec{
			Resources: resources,
		},
	}
}

var _ = Describe("createInstance", func() {
	It("injects gitops labels at metadata.labels and forwards the instance user values", func() {
		var created *catalogservice.CreateCatalogItemInstanceRequest
		itemSvc := &stubCatalogItemService{item: catalogItemWithFields([]catalogv1alpha1.FieldConfiguration{
			{Path: "metadata.name", Editable: boolPtr(true), Default: "decl-backend"},
		})}
		instSvc := &stubCatalogItemInstanceService{
			createFn: func(_ context.Context, req *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error) {
				created = req
				return &catalogv1alpha1.CatalogItemInstance{}, nil
			},
		}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			DisplayName:   "Decl App",
			Labels:        map[string]string{"team": "platform"},
			UserValues: []DesiredUserValue{
				{Resource: "backend", Path: "metadata.name", Value: "decl-backend-1"},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created).NotTo(BeNil())
		Expect(*created.ID).To(Equal("decl-app"))
		Expect(created.Spec.UserValues).To(Equal([]catalogv1alpha1.UserValue{
			{
				Resource: "backend",
				Path:     "metadata.labels",
				Value: map[string]string{
					"gitops.dcm.io/repository": "apps-repo",
					"gitops.dcm.io/commit":     "abc123",
					"team":                     "platform",
				},
			},
			{
				Resource: "backend",
				Path:     "metadata.name",
				Value:    "decl-backend-1",
			},
		}))
	})

	It("merges existing metadata.labels user values with gitops labels into one entry", func() {
		var created *catalogservice.CreateCatalogItemInstanceRequest
		itemSvc := &stubCatalogItemService{item: catalogItemWithFields([]catalogv1alpha1.FieldConfiguration{
			{Path: "metadata.name", Editable: boolPtr(true)},
		})}
		instSvc := &stubCatalogItemInstanceService{
			createFn: func(_ context.Context, req *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error) {
				created = req
				return &catalogv1alpha1.CatalogItemInstance{}, nil
			},
		}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			UserValues: []DesiredUserValue{
				{Resource: "backend", Path: "metadata.name", Value: "decl-backend-1"},
				{Resource: "backend", Path: "metadata.labels", Value: map[string]string{"tier": "frontend"}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Spec.UserValues).To(Equal([]catalogv1alpha1.UserValue{
			{
				Resource: "backend",
				Path:     "metadata.labels",
				Value: map[string]string{
					"tier":                     "frontend",
					"gitops.dcm.io/repository": "apps-repo",
					"gitops.dcm.io/commit":     "abc123",
				},
			},
			{
				Resource: "backend",
				Path:     "metadata.name",
				Value:    "decl-backend-1",
			},
		}))
	})

	It("rejects user values for unknown catalog resources", func() {
		itemSvc := &stubCatalogItemService{item: catalogItemWithFields(nil)}
		instSvc := &stubCatalogItemInstanceService{}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			UserValues: []DesiredUserValue{
				{Resource: "unknown-backend", Path: "metadata.labels", Value: map[string]string{"tier": "frontend"}},
			},
		})
		Expect(err).To(MatchError(ContainSubstring("user value resource not found in catalog item: unknown-backend")))
		Expect(errors.Is(err, catalogservice.ErrUserValueResourceNotFound)).To(BeTrue())
	})

	It("rejects metadata.labels user values with non-string entries", func() {
		itemSvc := &stubCatalogItemService{item: catalogItemWithFields(nil)}
		instSvc := &stubCatalogItemInstanceService{}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			UserValues: []DesiredUserValue{
				{Resource: "backend", Path: "metadata.labels", Value: map[string]any{"tier": 1}},
			},
		})
		Expect(err).To(MatchError(ContainSubstring(`resource backend metadata.labels: label "tier" must be a string, got int`)))
	})

	It("injects labels per resource and forwards user values to the matching resource only", func() {
		var created *catalogservice.CreateCatalogItemInstanceRequest
		itemSvc := &stubCatalogItemService{item: catalogItemWithResources([]catalogv1alpha1.CatalogResource{
			{
				Name: "backend",
				Fields: &[]catalogv1alpha1.FieldConfiguration{
					{Path: "metadata.name", Editable: boolPtr(true)},
					{Path: "metadata.labels", Editable: boolPtr(true)},
				},
			},
			{
				Name: "frontend",
				Fields: &[]catalogv1alpha1.FieldConfiguration{
					{Path: "metadata.name", Editable: boolPtr(true)},
					{Path: "metadata.labels", Editable: boolPtr(true)},
				},
			},
		})}
		instSvc := &stubCatalogItemInstanceService{
			createFn: func(_ context.Context, req *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error) {
				created = req
				return &catalogv1alpha1.CatalogItemInstance{}, nil
			},
		}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			UserValues: []DesiredUserValue{
				{Resource: "backend", Path: "metadata.name", Value: "decl-backend-1"},
				{Resource: "frontend", Path: "metadata.name", Value: "decl-frontend-1"},
				{Resource: "backend", Path: "metadata.labels", Value: map[string]string{"tier": "api"}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Spec.UserValues).To(Equal([]catalogv1alpha1.UserValue{
			{
				Resource: "backend",
				Path:     "metadata.labels",
				Value: map[string]string{
					"tier":                     "api",
					"gitops.dcm.io/repository": "apps-repo",
					"gitops.dcm.io/commit":     "abc123",
				},
			},
			{
				Resource: "frontend",
				Path:     "metadata.labels",
				Value: map[string]string{
					"gitops.dcm.io/repository": "apps-repo",
					"gitops.dcm.io/commit":     "abc123",
				},
			},
			{
				Resource: "backend",
				Path:     "metadata.name",
				Value:    "decl-backend-1",
			},
			{
				Resource: "frontend",
				Path:     "metadata.name",
				Value:    "decl-frontend-1",
			},
		}))
	})

	It("merges metadata.labels user values unmarshaled as map[string]any", func() {
		var created *catalogservice.CreateCatalogItemInstanceRequest
		itemSvc := &stubCatalogItemService{item: catalogItemWithFields(nil)}
		instSvc := &stubCatalogItemInstanceService{
			createFn: func(_ context.Context, req *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error) {
				created = req
				return &catalogv1alpha1.CatalogItemInstance{}, nil
			},
		}
		r := NewReconciler(nil, instSvc, itemSvc, nil)

		err := r.createInstance(context.Background(), "apps-repo", "abc123", DesiredInstance{
			Name:          "decl-app",
			CatalogItemID: "two-tier",
			UserValues: []DesiredUserValue{
				{Resource: "backend", Path: "metadata.labels", Value: map[string]any{"tier": "frontend"}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Spec.UserValues).To(Equal([]catalogv1alpha1.UserValue{
			{
				Resource: "backend",
				Path:     "metadata.labels",
				Value: map[string]string{
					"tier":                     "frontend",
					"gitops.dcm.io/repository": "apps-repo",
					"gitops.dcm.io/commit":     "abc123",
				},
			},
		}))
	})
})

type stubCatalogItemService struct {
	item *catalogv1alpha1.CatalogItem
	err  error
}

func (s *stubCatalogItemService) List(context.Context, catalogservice.CatalogItemListOptions) (*catalogservice.CatalogItemListResult, error) {
	return nil, nil
}

func (s *stubCatalogItemService) Create(context.Context, *catalogservice.CreateCatalogItemRequest) (*catalogv1alpha1.CatalogItem, error) {
	return nil, nil
}

func (s *stubCatalogItemService) Get(_ context.Context, _ string) (*catalogv1alpha1.CatalogItem, error) {
	return s.item, s.err
}

func (s *stubCatalogItemService) Update(context.Context, string, *catalogservice.UpdateCatalogItemRequest) (*catalogv1alpha1.CatalogItem, error) {
	return nil, nil
}
func (s *stubCatalogItemService) Delete(context.Context, string) error { return nil }

type stubCatalogItemInstanceService struct {
	createFn func(context.Context, *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error)
}

func (s *stubCatalogItemInstanceService) List(context.Context, catalogservice.CatalogItemInstanceListOptions) (*catalogservice.CatalogItemInstanceListResult, error) {
	return nil, nil
}

func (s *stubCatalogItemInstanceService) Create(ctx context.Context, req *catalogservice.CreateCatalogItemInstanceRequest) (*catalogv1alpha1.CatalogItemInstance, error) {
	if s.createFn != nil {
		return s.createFn(ctx, req)
	}
	return &catalogv1alpha1.CatalogItemInstance{}, nil
}

func (s *stubCatalogItemInstanceService) Get(context.Context, string) (*catalogv1alpha1.CatalogItemInstance, error) {
	return nil, nil
}
func (s *stubCatalogItemInstanceService) Delete(context.Context, string) error { return nil }
func (s *stubCatalogItemInstanceService) Rehydrate(context.Context, string) (*catalogv1alpha1.CatalogItemInstance, error) {
	return nil, nil
}
