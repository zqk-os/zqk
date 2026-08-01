package ontology

import "github.com/lanceman/zqk/pkg/objects"

func SystemOntology() Ontology {
	return Ontology{
		ID:      "zqk-system-ontology",
		Version: "1.0",
		Classes: map[string]Class{
			"goal": {
				Name: "goal",
				Properties: map[string]Property{
					objects.FieldKeyID:             {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:           {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:          {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:         {Name: "status", Type: "string", Required: true},
					objects.FieldKeyWorkstreamRefs: {Name: "workstream_refs", Type: "list", Required: false},
					objects.FieldKeyMilestoneRefs:  {Name: "milestone_refs", Type: "list", Required: false},
				},
			},
			"milestone": {
				Name: "milestone",
				Properties: map[string]Property{
					objects.FieldKeyID:              {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:            {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:           {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:          {Name: "status", Type: "string", Required: true},
					objects.FieldKeyWorkstreamRefs:  {Name: "workstream_refs", Type: "list", Required: false},
					objects.FieldKeyGoalRefs:        {Name: "goal_refs", Type: "list", Required: false},
					objects.FieldKeyCriteriaRefs:    {Name: "criteria_refs", Type: "list", Required: false},
					objects.FieldKeyRequirementRefs: {Name: "requirement_refs", Type: "list", Required: false},
				},
			},
			"workstream": {
				Name: "workstream",
				Properties: map[string]Property{
					objects.FieldKeyID:            {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:          {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:         {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:        {Name: "status", Type: "string", Required: true},
					objects.FieldKeyOwnerRef:      {Name: "owner_ref", Type: "string", Required: false},
					objects.FieldKeyGoalRefs:      {Name: "goal_refs", Type: "list", Required: false},
					objects.FieldKeyMilestoneRefs: {Name: "milestone_refs", Type: "list", Required: false},
				},
			},
			"priority_plan": {
				Name: "priority_plan",
				Properties: map[string]Property{
					objects.FieldKeyID:            {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:          {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:         {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:        {Name: "status", Type: "string", Required: true},
					objects.FieldKeyWorkstreamRef: {Name: "workstream_ref", Type: "string", Required: false},
				},
			},
			"backlog_item": {
				Name: "backlog_item",
				Properties: map[string]Property{
					objects.FieldKeyID:              {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:            {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:           {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:          {Name: "status", Type: "string", Required: true},
					objects.FieldKeyPriorityPlanRef: {Name: "priority_plan_ref", Type: "string", Required: false},
					objects.FieldKeyMilestoneRefs:   {Name: "milestone_refs", Type: "list", Required: false},
					objects.FieldKeyGoalRefs:        {Name: "goal_refs", Type: "list", Required: false},
					objects.FieldKeyRequirementRefs: {Name: "requirement_refs", Type: "list", Required: false},
				},
			},
			"requirement": {
				Name: "requirement",
				Properties: map[string]Property{
					objects.FieldKeyID:            {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:          {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:         {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:        {Name: "status", Type: "string", Required: true},
					objects.FieldKeyMilestoneRefs: {Name: "milestone_refs", Type: "list", Required: false},
					objects.FieldKeyGoalRefs:      {Name: "goal_refs", Type: "list", Required: false},
					objects.FieldKeyTestCaseRefs:  {Name: "test_case_refs", Type: "list", Required: false},
				},
			},
			"criteria": {
				Name: "criteria",
				Properties: map[string]Property{
					objects.FieldKeyID:              {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:            {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:           {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:          {Name: "status", Type: "string", Required: true},
					objects.FieldKeyMilestoneRefs:   {Name: "milestone_refs", Type: "list", Required: false},
					objects.FieldKeyRequirementRefs: {Name: "requirement_refs", Type: "list", Required: false},
				},
			},
			"test_case": {
				Name: "test_case",
				Properties: map[string]Property{
					objects.FieldKeyID:              {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:            {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:           {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:          {Name: "status", Type: "string", Required: true},
					objects.FieldKeyRequirementRefs: {Name: "requirement_refs", Type: "list", Required: false},
					objects.FieldKeyMilestoneRefs:   {Name: "milestone_refs", Type: "list", Required: false},
				},
			},
			"roadmap": {
				Name: "roadmap",
				Properties: map[string]Property{
					objects.FieldKeyID:             {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:           {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:          {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:         {Name: "status", Type: "string", Required: true},
					objects.FieldKeyWorkstreamRefs: {Name: "workstream_refs", Type: "list", Required: false},
				},
			},
			"mission": {
				Name: "mission",
				Properties: map[string]Property{
					objects.FieldKeyID:     {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:   {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:  {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus: {Name: "status", Type: "string", Required: true},
				},
			},
			objects.FieldKeyVision: {
				Name: "vision",
				Properties: map[string]Property{
					objects.FieldKeyID:     {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:   {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:  {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus: {Name: "status", Type: "string", Required: true},
				},
			},
			"decision": {
				Name: "decision",
				Properties: map[string]Property{
					objects.FieldKeyID:     {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:   {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:  {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus: {Name: "status", Type: "string", Required: true},
				},
			},
			objects.FieldKeyComponent: {
				Name: "component",
				Properties: map[string]Property{
					objects.FieldKeyID:                  {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:                {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:               {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus:              {Name: "status", Type: "string", Required: true},
					objects.FieldKeyParentComponentRefs: {Name: "parent_component_refs", Type: "list", Required: false},
					objects.FieldKeyChildComponentRefs:  {Name: "child_component_refs", Type: "list", Required: false},
					objects.FieldKeyObjectRef:           {Name: "object_ref", Type: "string", Required: false},
				},
			},
			"account": {
				Name: "account",
				Properties: map[string]Property{
					objects.FieldKeyID:     {Name: "id", Type: "string", Required: true},
					objects.FieldKeyKind:   {Name: "kind", Type: "string", Required: true},
					objects.FieldKeyTitle:  {Name: "title", Type: "string", Required: true},
					objects.FieldKeyStatus: {Name: "status", Type: "string", Required: true},
				},
			},
		},
	}
}
