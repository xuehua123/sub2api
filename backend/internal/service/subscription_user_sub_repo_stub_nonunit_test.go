//go:build !unit

package service

import "context"

func (s *subscriptionUserSubRepoStub) Delete(_ context.Context, subscriptionID int64) error {
	sub := s.byID[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	s.deleteCalls++
	delete(s.byID, subscriptionID)
	delete(s.byUserGroup, s.key(sub.UserID, sub.GroupID))
	return nil
}
