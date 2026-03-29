Feature: Loan origination workflow

  Scenario: Low risk loan is auto-approved
    Given a case type "loan_application" is registered
    And a workflow "loan_origination" is deployed
    When a case is created with:
      | field                  | value    |
      | applicant.company_name | Acme Ltd |
      | loan.amount            | 8000     |
    And the intake step completes
    And the agent "risk_assessment" returns:
      | score | risk_level | confidence |
      | 85    | low        | 0.92       |
    Then the case should proceed to "generate_offer"
    And no human task should be created

  Scenario: High value loan requires underwriter review
    Given a case type "loan_application" is registered
    And a workflow "loan_origination" is deployed
    When a case is created with:
      | field       | value   |
      | loan.amount | 150000  |
    And the risk assessment completes with score 65
    And the auto_decision routes to "underwriter_review"
    Then a task should be created for role "underwriter"
    And the task SLA should be 24 hours

  Scenario: Underwriter refers to senior
    Given a case with an active underwriter review task
    When the underwriter submits outcome "referred"
    Then a task should be created for role "senior_underwriter"
    And the task SLA should be 48 hours
    And the underwriter review task should be completed
