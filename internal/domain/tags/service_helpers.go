package tags

import (
	pb "github.com/FACorreiaa/loci-proto/modules/tags/generated"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (svc *Service) convertTagToProto(tag *Tags) *pb.PersonalTag {
	if tag == nil {
		return nil
	}

	protoTag := &pb.PersonalTag{
		Id:      tag.ID.String(),
		Name:    tag.Name,
		TagType: tag.TagType,
	}

	if tag.Description != nil {
		protoTag.Description = *tag.Description
	}

	if tag.Source != nil {
		protoTag.Source = *tag.Source
	}

	if !tag.CreatedAt.IsZero() {
		protoTag.CreatedAt = timestamppb.New(tag.CreatedAt)
	}

	if tag.UpdatedAt != nil {
		protoTag.UpdatedAt = timestamppb.New(*tag.UpdatedAt)
	}

	return protoTag
}

func (svc *Service) convertPersonalTagToProto(tag *PersonalTag) *pb.PersonalTag {
	if tag == nil {
		return nil
	}

	protoTag := &pb.PersonalTag{
		Id:      tag.ID.String(),
		UserId:  tag.UserID.String(),
		Name:    tag.Name,
		TagType: tag.TagType,
		Source:  tag.Source,
	}

	if tag.Description != nil {
		protoTag.Description = *tag.Description
	}

	if !tag.CreatedAt.IsZero() {
		protoTag.CreatedAt = timestamppb.New(tag.CreatedAt)
	}

	if tag.UpdatedAt != nil {
		protoTag.UpdatedAt = timestamppb.New(*tag.UpdatedAt)
	}

	return protoTag
}
